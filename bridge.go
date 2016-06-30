package vsolver

import (
	"fmt"
	"os"
	"sort"
)

// sourceBridges provide an adapter to SourceManagers that tailor operations
// for a single solve run.
type sourceBridge interface {
	getProjectInfo(pa atom) (Manifest, Lock, error)
	listVersions(id ProjectIdentifier) ([]Version, error)
	pairRevision(id ProjectIdentifier, r Revision) []Version
	pairVersion(id ProjectIdentifier, v UnpairedVersion) PairedVersion
	repoExists(id ProjectIdentifier) (bool, error)
	vendorCodeExists(id ProjectIdentifier) (bool, error)
	matches(id ProjectIdentifier, c Constraint, v Version) bool
	matchesAny(id ProjectIdentifier, c1, c2 Constraint) bool
	intersect(id ProjectIdentifier, c1, c2 Constraint) Constraint
	listPackages(id ProjectIdentifier, v Version) (PackageTree, error)
	computeRootReach(path string) ([]string, error)
	verifyRoot(path string) error
	deduceRemoteRepo(path string) (*remoteRepo, error)
}

func newBridge(name ProjectName, root string, sm SourceManager, downgrade bool) sourceBridge {
	return &bridge{
		sm:       sm,
		sortdown: downgrade,
		name:     name,
		root:     root,
		vlists:   make(map[ProjectName][]Version),
	}
}

// bridge is an adapter around a proper SourceManager. It provides localized
// caching that's tailored to the requirements of a particular solve run.
//
// It also performs transformations between ProjectIdentifiers, which is what
// the solver primarily deals in, and ProjectName, which is what the
// SourceManager primarily deals in. This separation is helpful because it keeps
// the complexities of deciding what a particular name "means" entirely within
// the solver, while the SourceManager can traffic exclusively in
// globally-unique network names.
//
// Finally, it provides authoritative version/constraint operations, ensuring
// that any possible approach to a match - even those not literally encoded in
// the inputs - is achieved.
type bridge struct {
	// The underlying, adapted-to SourceManager
	sm SourceManager

	// Direction to sort the version list. False indicates sorting for upgrades;
	// true for downgrades.
	sortdown bool

	// The name of the root project we're operating on. Used to redirect some
	// calls that would ordinarily go to the SourceManager to a root-specific
	// logical path, instead.
	name ProjectName

	// The path to the base directory of the root project.
	root string

	// Simple, local cache of the root's PackageTree
	crp *struct {
		ptree PackageTree
		err   error
	}

	// Map of project root name to their available version list. This cache is
	// layered on top of the proper SourceManager's cache; the only difference
	// is that this keeps the versions sorted in the direction required by the
	// current solve run
	vlists map[ProjectName][]Version
}

func (b *bridge) getProjectInfo(pa atom) (Manifest, Lock, error) {
	return b.sm.GetProjectInfo(ProjectName(pa.id.netName()), pa.v)
}

func (b *bridge) key(id ProjectIdentifier) ProjectName {
	k := ProjectName(id.NetworkName)
	if k == "" {
		k = id.LocalName
	}

	return k
}

func (b *bridge) listVersions(id ProjectIdentifier) ([]Version, error) {
	k := b.key(id)

	if vl, exists := b.vlists[k]; exists {
		return vl, nil
	}

	vl, err := b.sm.ListVersions(k)
	// TODO cache errors, too?
	if err != nil {
		return nil, err
	}

	if b.sortdown {
		sort.Sort(downgradeVersionSorter(vl))
	} else {
		sort.Sort(upgradeVersionSorter(vl))
	}

	b.vlists[k] = vl
	return vl, nil
}

func (b *bridge) repoExists(id ProjectIdentifier) (bool, error) {
	k := b.key(id)
	return b.sm.RepoExists(k)
}

func (b *bridge) vendorCodeExists(id ProjectIdentifier) (bool, error) {
	k := b.key(id)
	return b.sm.VendorCodeExists(k)
}

func (b *bridge) pairVersion(id ProjectIdentifier, v UnpairedVersion) PairedVersion {
	vl, err := b.listVersions(id)
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
	vl, err := b.listVersions(id)
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

// computeRootReach is a specialized, less stringent version of listExternal
// that allows for a bit of fuzziness in the source inputs.
//
// Specifically, we need to:
//  - Analyze test-type files as well as typical source files
//  - Make a best-effort attempt even if the code doesn't compile
//  - Include main packages in the analysis
//
// Perhaps most important is that we don't want to have the results of this
// analysis be in any permanent cache, and we want to read directly from our
// potentially messy root project source location on disk. Together, this means
// that we can't ask the real SourceManager to do it.
func (b *bridge) computeRootReach(path string) ([]string, error) {
	// TODO i now cannot remember the reasons why i thought being less stringent
	// in the analysis was OK. so, for now, we just compute list of
	// externally-touched packages.

	if b.crp == nil {
		ptree, err := listPackages(b.root, string(b.name))
		b.crp = &struct {
			ptree PackageTree
			err   error
		}{
			ptree: ptree,
			err:   err,
		}
	}
	if b.crp.err != nil {
		return nil, b.crp.err
	}

	return b.crp.ptree.ListExternalImports(true, true)
}

// listPackages lists all the packages contained within the given project at a
// particular version.
//
// The root project is handled separately, as the source manager isn't
// responsible for that code.
func (b *bridge) listPackages(id ProjectIdentifier, v Version) (PackageTree, error) {
	if id.LocalName != b.name {
		// FIXME if we're aliasing here, the returned PackageTree will have
		// unaliased import paths, which is super not correct
		return b.sm.ListPackages(b.key(id), v)
	}
	if b.crp == nil {
		ptree, err := listPackages(b.root, string(b.name))
		b.crp = &struct {
			ptree PackageTree
			err   error
		}{
			ptree: ptree,
			err:   err,
		}
	}

	return b.crp.ptree, b.crp.err
}

// verifyRoot ensures that the provided path to the project root is in good
// working condition. This check is made only once, at the beginning of a solve
// run.
func (b *bridge) verifyRoot(path string) error {
	if fi, err := os.Stat(path); err != nil {
		return badOptsFailure(fmt.Sprintf("Could not read project root (%s): %s", path, err))
	} else if !fi.IsDir() {
		return badOptsFailure(fmt.Sprintf("Project root (%s) is a file, not a directory.", path))
	}

	return nil
}

// deduceRemoteRepo deduces certain network-oriented properties about an import
// path.
func (b *bridge) deduceRemoteRepo(path string) (*remoteRepo, error) {
	return deduceRemoteRepo(path)
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

type upgradeVersionSorter []Version
type downgradeVersionSorter []Version

func (vs upgradeVersionSorter) Len() int {
	return len(vs)
}

func (vs upgradeVersionSorter) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}

func (vs downgradeVersionSorter) Len() int {
	return len(vs)
}

func (vs downgradeVersionSorter) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}

func (vs upgradeVersionSorter) Less(i, j int) bool {
	l, r := vs[i], vs[j]

	if tl, ispair := l.(versionPair); ispair {
		l = tl.v
	}
	if tr, ispair := r.(versionPair); ispair {
		r = tr.v
	}

	switch compareVersionType(l, r) {
	case -1:
		return true
	case 1:
		return false
	case 0:
		break
	default:
		panic("unreachable")
	}

	switch l.(type) {
	// For these, now nothing to do but alpha sort
	case Revision, branchVersion, plainVersion:
		return l.String() < r.String()
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lsv, rsv := l.(semVersion).sv, r.(semVersion).sv
	lpre, rpre := lsv.Prerelease() == "", rsv.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return lsv.GreaterThan(rsv)
}

func (vs downgradeVersionSorter) Less(i, j int) bool {
	l, r := vs[i], vs[j]

	if tl, ispair := l.(versionPair); ispair {
		l = tl.v
	}
	if tr, ispair := r.(versionPair); ispair {
		r = tr.v
	}

	switch compareVersionType(l, r) {
	case -1:
		return true
	case 1:
		return false
	case 0:
		break
	default:
		panic("unreachable")
	}

	switch l.(type) {
	// For these, now nothing to do but alpha
	case Revision, branchVersion, plainVersion:
		return l.String() < r.String()
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lsv, rsv := l.(semVersion).sv, r.(semVersion).sv
	lpre, rpre := lsv.Prerelease() == "", rsv.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return lsv.LessThan(rsv)
}
