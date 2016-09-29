package gps

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/Masterminds/semver"
)

// sourceBridges provide an adapter to SourceManagers that tailor operations
// for a single solve run.
type sourceBridge interface {
	SourceManager // composes SourceManager
	verifyRootDir(path string) error
	pairRevision(id ProjectIdentifier, r Revision) []Version
	pairVersion(id ProjectIdentifier, v UnpairedVersion) PairedVersion
	vendorCodeExists(id ProjectIdentifier) (bool, error)
	matches(id ProjectIdentifier, c Constraint, v Version) bool
	matchesAny(id ProjectIdentifier, c1, c2 Constraint) bool
	intersect(id ProjectIdentifier, c1, c2 Constraint) Constraint
	breakLock()
}

// bridge is an adapter around a proper SourceManager. It provides localized
// caching that's tailored to the requirements of a particular solve run.
//
// Finally, it provides authoritative version/constraint operations, ensuring
// that any possible approach to a match - even those not literally encoded in
// the inputs - is achieved.
type bridge struct {
	// The underlying, adapted-to SourceManager
	sm SourceManager

	// The solver which we're assisting.
	//
	// The link between solver and bridge is circular, which is typically a bit
	// awkward, but the bridge needs access to so many of the input arguments
	// held by the solver that it ends up being easier and saner to do this.
	s *solver

	// Simple, local cache of the root's PackageTree
	crp *struct {
		ptree PackageTree
		err   error
	}

	// Map of project root name to their available version list. This cache is
	// layered on top of the proper SourceManager's cache; the only difference
	// is that this keeps the versions sorted in the direction required by the
	// current solve run
	vlists map[ProjectIdentifier][]Version

	// Indicates whether lock breaking has already been run
	lockbroken int32
}

// Global factory func to create a bridge. This exists solely to allow tests to
// override it with a custom bridge and sm.
var mkBridge func(*solver, SourceManager) sourceBridge = func(s *solver, sm SourceManager) sourceBridge {
	return &bridge{
		sm:     sm,
		s:      s,
		vlists: make(map[ProjectIdentifier][]Version),
	}
}

func (b *bridge) GetManifestAndLock(id ProjectIdentifier, v Version) (Manifest, Lock, error) {
	if id.ProjectRoot == ProjectRoot(b.s.rpt.ImportRoot) {
		return b.s.rm, b.s.rl, nil
	}
	return b.sm.GetManifestAndLock(id, v)
}

func (b *bridge) AnalyzerInfo() (string, *semver.Version) {
	return b.sm.AnalyzerInfo()
}

func (b *bridge) ListVersions(id ProjectIdentifier) ([]Version, error) {
	if vl, exists := b.vlists[id]; exists {
		return vl, nil
	}

	vl, err := b.sm.ListVersions(id)
	// TODO(sdboyer) cache errors, too?
	if err != nil {
		return nil, err
	}

	if b.s.params.Downgrade {
		SortForDowngrade(vl)
	} else {
		SortForUpgrade(vl)
	}

	b.vlists[id] = vl
	return vl, nil
}

func (b *bridge) RevisionPresentIn(id ProjectIdentifier, r Revision) (bool, error) {
	return b.sm.RevisionPresentIn(id, r)
}

func (b *bridge) SourceExists(id ProjectIdentifier) (bool, error) {
	return b.sm.SourceExists(id)
}

func (b *bridge) vendorCodeExists(id ProjectIdentifier) (bool, error) {
	fi, err := os.Stat(filepath.Join(b.s.params.RootDir, "vendor", string(id.ProjectRoot)))
	if err != nil {
		return false, err
	} else if fi.IsDir() {
		return true, nil
	}

	return false, nil
}

func (b *bridge) pairVersion(id ProjectIdentifier, v UnpairedVersion) PairedVersion {
	vl, err := b.ListVersions(id)
	if err != nil {
		return nil
	}

	// doing it like this is a bit sloppy
	for _, v2 := range vl {
		if p, ok := v2.(PairedVersion); ok {
			if p.Matches(v) {
				return p
			}
		}
	}

	return nil
}

func (b *bridge) pairRevision(id ProjectIdentifier, r Revision) []Version {
	vl, err := b.ListVersions(id)
	if err != nil {
		return nil
	}

	p := []Version{r}
	// doing it like this is a bit sloppy
	for _, v2 := range vl {
		if pv, ok := v2.(PairedVersion); ok {
			if pv.Matches(r) {
				p = append(p, pv)
			}
		}
	}

	return p
}

// matches performs a typical match check between the provided version and
// constraint. If that basic check fails and the provided version is incomplete
// (e.g. an unpaired version or bare revision), it will attempt to gather more
// information on one or the other and re-perform the comparison.
func (b *bridge) matches(id ProjectIdentifier, c2 Constraint, v Version) bool {
	if c2.Matches(v) {
		return true
	}

	// There's a wide field of possible ways that pairing might result in a
	// match. For each possible type of version, start by carving out all the
	// cases where the constraint would have provided an authoritative match
	// result.
	switch tv := v.(type) {
	case PairedVersion:
		switch tc := c2.(type) {
		case PairedVersion, Revision, noneConstraint:
			// These three would all have been authoritative matches
			return false
		case UnpairedVersion:
			// Only way paired and unpaired could match is if they share an
			// underlying rev
			pv := b.pairVersion(id, tc)
			if pv == nil {
				return false
			}
			return pv.Matches(v)
		case semverConstraint:
			// Have to check all the possible versions for that rev to see if
			// any match the semver constraint
			for _, pv := range b.pairRevision(id, tv.Underlying()) {
				if tc.Matches(pv) {
					return true
				}
			}
			return false
		}

	case Revision:
		switch tc := c2.(type) {
		case PairedVersion, Revision, noneConstraint:
			// These three would all have been authoritative matches
			return false
		case UnpairedVersion:
			// Only way paired and unpaired could match is if they share an
			// underlying rev
			pv := b.pairVersion(id, tc)
			if pv == nil {
				return false
			}
			return pv.Matches(v)
		case semverConstraint:
			// Have to check all the possible versions for the rev to see if
			// any match the semver constraint
			for _, pv := range b.pairRevision(id, tv) {
				if tc.Matches(pv) {
					return true
				}
			}
			return false
		}

	// UnpairedVersion as input has the most weird cases. It's also the one
	// we'll probably see the least
	case UnpairedVersion:
		switch tc := c2.(type) {
		case noneConstraint:
			// obviously
			return false
		case Revision, PairedVersion:
			// Easy case for both - just pair the uv and see if it matches the revision
			// constraint
			pv := b.pairVersion(id, tv)
			if pv == nil {
				return false
			}
			return tc.Matches(pv)
		case UnpairedVersion:
			// Both are unpaired versions. See if they share an underlying rev.
			pv := b.pairVersion(id, tv)
			if pv == nil {
				return false
			}

			pc := b.pairVersion(id, tc)
			if pc == nil {
				return false
			}
			return pc.Matches(pv)

		case semverConstraint:
			// semverConstraint can't ever match a rev, but we do need to check
			// if any other versions corresponding to this rev work.
			pv := b.pairVersion(id, tv)
			if pv == nil {
				return false
			}

			for _, ttv := range b.pairRevision(id, pv.Underlying()) {
				if c2.Matches(ttv) {
					return true
				}
			}
			return false
		}
	default:
		panic("unreachable")
	}

	return false
}

// matchesAny is the authoritative version of Constraint.MatchesAny.
func (b *bridge) matchesAny(id ProjectIdentifier, c1, c2 Constraint) bool {
	if c1.MatchesAny(c2) {
		return true
	}

	// This approach is slightly wasteful, but just SO much less verbose, and
	// more easily understood.
	var uc1, uc2 Constraint
	if v1, ok := c1.(Version); ok {
		uc1 = b.vtu(id, v1)
	} else {
		uc1 = c1
	}

	if v2, ok := c2.(Version); ok {
		uc2 = b.vtu(id, v2)
	} else {
		uc2 = c2
	}

	return uc1.MatchesAny(uc2)
}

// intersect is the authoritative version of Constraint.Intersect.
func (b *bridge) intersect(id ProjectIdentifier, c1, c2 Constraint) Constraint {
	rc := c1.Intersect(c2)
	if rc != none {
		return rc
	}

	// This approach is slightly wasteful, but just SO much less verbose, and
	// more easily understood.
	var uc1, uc2 Constraint
	if v1, ok := c1.(Version); ok {
		uc1 = b.vtu(id, v1)
	} else {
		uc1 = c1
	}

	if v2, ok := c2.(Version); ok {
		uc2 = b.vtu(id, v2)
	} else {
		uc2 = c2
	}

	return uc1.Intersect(uc2)
}

// vtu creates a versionTypeUnion for the provided version.
//
// This union may (and typically will) end up being nothing more than the single
// input version, but creating a versionTypeUnion guarantees that 'local'
// constraint checks (direct method calls) are authoritative.
func (b *bridge) vtu(id ProjectIdentifier, v Version) versionTypeUnion {
	switch tv := v.(type) {
	case Revision:
		return versionTypeUnion(b.pairRevision(id, tv))
	case PairedVersion:
		return versionTypeUnion(b.pairRevision(id, tv.Underlying()))
	case UnpairedVersion:
		pv := b.pairVersion(id, tv)
		if pv == nil {
			return versionTypeUnion{tv}
		}

		return versionTypeUnion(b.pairRevision(id, pv.Underlying()))
	}

	return nil
}

// listPackages lists all the packages contained within the given project at a
// particular version.
//
// The root project is handled separately, as the source manager isn't
// responsible for that code.
func (b *bridge) ListPackages(id ProjectIdentifier, v Version) (PackageTree, error) {
	if id.ProjectRoot == ProjectRoot(b.s.rpt.ImportRoot) {
		panic("should never call ListPackages on root project")
	}

	return b.sm.ListPackages(id, v)
}

func (b *bridge) ExportProject(id ProjectIdentifier, v Version, path string) error {
	panic("bridge should never be used to ExportProject")
}

// verifyRoot ensures that the provided path to the project root is in good
// working condition. This check is made only once, at the beginning of a solve
// run.
func (b *bridge) verifyRootDir(path string) error {
	if fi, err := os.Stat(path); err != nil {
		return badOptsFailure(fmt.Sprintf("could not read project root (%s): %s", path, err))
	} else if !fi.IsDir() {
		return badOptsFailure(fmt.Sprintf("project root (%s) is a file, not a directory", path))
	}

	return nil
}

func (b *bridge) DeduceProjectRoot(ip string) (ProjectRoot, error) {
	return b.sm.DeduceProjectRoot(ip)
}

// breakLock is called when the solver has to break a version recorded in the
// lock file. It prefetches all the projects in the solver's lock , so that the
// information is already on hand if/when the solver needs it.
//
// Projects that have already been selected are skipped, as it's generally unlikely that the
// solver will have to backtrack through and fully populate their version queues.
func (b *bridge) breakLock() {
	// No real conceivable circumstance in which multiple calls are made to
	// this, but being that this is the entrance point to a bunch of async work,
	// protect it with an atomic CAS in case things change in the future.
	if !atomic.CompareAndSwapInt32(&b.lockbroken, 0, 1) {
		return
	}

	for _, lp := range b.s.rl.Projects() {
		if _, is := b.s.sel.selected(lp.pi); !is {
			// ListPackages guarantees that all the necessary network work will
			// be done, so go with that
			//
			// TODO(sdboyer) use this as an opportunity to detect
			// inconsistencies between upstream and the lock (e.g., moved tags)?
			pi, v := lp.pi, lp.Version()
			go func() {
				// Sync first
				b.sm.SyncSourceFor(pi)
				// Preload the package info for the locked version, too, as
				// we're more likely to need that
				b.sm.ListPackages(pi, v)
			}()
		}
	}
}

func (b *bridge) SyncSourceFor(id ProjectIdentifier) error {
	return b.sm.SyncSourceFor(id)
}

// versionTypeUnion represents a set of versions that are, within the scope of
// this solver run, equivalent.
//
// The simple case here is just a pair - a normal version plus its underlying
// revision - but if a tag or branch point at the same rev, then we consider
// them equivalent. Again, however, this equivalency is short-lived; it must be
// re-assessed during every solver run.
//
// The union members are treated as being OR'd together:  all constraint
// operations attempt each member, and will take the most open/optimistic
// answer.
//
// This technically does allow tags to match branches - something we
// otherwise try hard to avoid - but because the original input constraint never
// actually changes (and is never written out in the Result), there's no harmful
// case of a user suddenly riding a branch when they expected a fixed tag.
type versionTypeUnion []Version

// This should generally not be called, but is required for the interface. If it
// is called, we have a bigger problem (the type has escaped the solver); thus,
// panic.
func (av versionTypeUnion) String() string {
	panic("versionTypeUnion should never be turned into a string; it is solver internal-only")
}

// This should generally not be called, but is required for the interface. If it
// is called, we have a bigger problem (the type has escaped the solver); thus,
// panic.
func (av versionTypeUnion) Type() string {
	panic("versionTypeUnion should never need to answer a Type() call; it is solver internal-only")
}

// Matches takes a version, and returns true if that version matches any version
// contained in the union.
//
// This DOES allow tags to match branches, albeit indirectly through a revision.
func (av versionTypeUnion) Matches(v Version) bool {
	av2, oav := v.(versionTypeUnion)

	for _, v1 := range av {
		if oav {
			for _, v2 := range av2 {
				if v1.Matches(v2) {
					return true
				}
			}
		} else if v1.Matches(v) {
			return true
		}
	}

	return false
}

// MatchesAny returns true if any of the contained versions (which are also
// constraints) in the union successfully MatchAny with the provided
// constraint.
func (av versionTypeUnion) MatchesAny(c Constraint) bool {
	av2, oav := c.(versionTypeUnion)

	for _, v1 := range av {
		if oav {
			for _, v2 := range av2 {
				if v1.MatchesAny(v2) {
					return true
				}
			}
		} else if v1.MatchesAny(c) {
			return true
		}
	}

	return false
}

// Intersect takes a constraint, and attempts to intersect it with all the
// versions contained in the union until one returns non-none. If that never
// happens, then none is returned.
//
// In order to avoid weird version floating elsewhere in the solver, the union
// always returns the input constraint. (This is probably obviously correct, but
// is still worth noting.)
func (av versionTypeUnion) Intersect(c Constraint) Constraint {
	av2, oav := c.(versionTypeUnion)

	for _, v1 := range av {
		if oav {
			for _, v2 := range av2 {
				if rc := v1.Intersect(v2); rc != none {
					return rc
				}
			}
		} else if rc := v1.Intersect(c); rc != none {
			return rc
		}
	}

	return none
}

func (av versionTypeUnion) _private() {}
