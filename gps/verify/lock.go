package verify

import (
	"github.com/armon/go-radix"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/paths"
	"github.com/golang/dep/gps/pkgtree"
)

// VerifiableProject composes a LockedProject to indicate what the hash digest
// of a file tree for that LockedProject should be, given the PruneOptions and
// the list of packages.
type VerifiableProject struct {
	gps.LockedProject
	PruneOpts gps.PruneOptions
	Digest    pkgtree.VersionedDigest
}

type LockDiff struct{}

type lockUnsatisfy uint8

const (
	missingFromLock lockUnsatisfy = iota
	inAdditionToLock
)

type constraintMismatch struct {
	c gps.Constraint
	v gps.Version
}

type constraintMismatches map[gps.ProjectRoot]constraintMismatch

type LockSatisfaction struct {
	nolock                  bool
	missingPkgs, excessPkgs []string
	badovr, badconstraint   constraintMismatches
}

// Passed is a shortcut method to check if any problems with the evaluted lock
// were identified.
func (ls LockSatisfaction) Passed() bool {
	if ls.nolock {
		return false
	}

	if len(ls.missingPkgs) > 0 {
		return false
	}

	if len(ls.excessPkgs) > 0 {
		return false
	}

	if len(ls.badovr) > 0 {
		return false
	}

	if len(ls.badconstraint) > 0 {
		return false
	}

	return true
}

func (ls LockSatisfaction) MissingPackages() []string {
	return ls.missingPkgs
}

func (ls LockSatisfaction) ExcessPackages() []string {
	return ls.excessPkgs
}

func (ls LockSatisfaction) UnmatchedOverrides() map[gps.ProjectRoot]constraintMismatch {
	return ls.badovr
}

func (ls LockSatisfaction) UnmatchedConstraints() map[gps.ProjectRoot]constraintMismatch {
	return ls.badconstraint
}

func findEffectualConstraints(m gps.Manifest, imports map[string]bool) map[string]bool {
	eff := make(map[string]bool)
	xt := radix.New()

	for pr, _ := range m.DependencyConstraints() {
		xt.Insert(string(pr), nil)
	}

	for imp := range imports {
		if root, _, has := xt.LongestPrefix(imp); has {
			eff[root] = true
		}
	}

	return eff
}

// LockSatisfiesInputs determines whether the provided Lock satisfies all the
// requirements indicated by the inputs (RootManifest and PackageTree).
//
// The second parameter is expected to be the list of imports that were used to
// generate the input Lock. Without this explicit list, it is not possible to
// compute package imports that may have been removed. Figuring out that
// negative space would require exploring the entire graph to ensure there are
// no in-edges for particular imports.
func LockSatisfiesInputs(l gps.Lock, oldimports []string, m gps.RootManifest, rpt pkgtree.PackageTree) LockSatisfaction {
	if l == nil {
		return LockSatisfaction{nolock: true}
	}

	var ig *pkgtree.IgnoredRuleset
	var req map[string]bool
	if m != nil {
		ig = m.IgnoredPackages()
		req = m.RequiredPackages()
	}

	rm, _ := rpt.ToReachMap(true, true, false, ig)
	reach := rm.FlattenFn(paths.IsStandardImportPath)

	inlock := make(map[string]bool, len(oldimports))
	ininputs := make(map[string]bool, len(reach)+len(req))
	pkgDiff := make(map[string]lockUnsatisfy)

	for _, imp := range reach {
		ininputs[imp] = true
	}

	for imp := range req {
		ininputs[imp] = true
	}

	for _, imp := range oldimports {
		inlock[imp] = true
	}

	lsat := LockSatisfaction{
		badovr:        make(constraintMismatches),
		badconstraint: make(constraintMismatches),
	}

	for ip := range ininputs {
		if !inlock[ip] {
			pkgDiff[ip] = missingFromLock
		} else {
			// So we don't have to revisit it below
			delete(inlock, ip)
		}
	}

	for ip := range inlock {
		if !ininputs[ip] {
			pkgDiff[ip] = inAdditionToLock
		}
	}

	for ip, typ := range pkgDiff {
		if typ == missingFromLock {
			lsat.missingPkgs = append(lsat.missingPkgs, ip)
		} else {
			lsat.excessPkgs = append(lsat.excessPkgs, ip)
		}
	}

	eff := findEffectualConstraints(m, ininputs)
	ovr := m.Overrides()
	constraints := m.DependencyConstraints()

	for _, lp := range l.Projects() {
		pr := lp.Ident().ProjectRoot

		if pp, has := ovr[pr]; has && !pp.Constraint.Matches(lp.Version()) {
			lsat.badovr[pr] = constraintMismatch{
				c: pp.Constraint,
				v: lp.Version(),
			}
		}

		if pp, has := constraints[pr]; has && eff[string(pr)] && !pp.Constraint.Matches(lp.Version()) {
			lsat.badconstraint[pr] = constraintMismatch{
				c: pp.Constraint,
				v: lp.Version(),
			}
		}
	}

	return lsat
}
