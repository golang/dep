package gps

import (
	"sort"

	"github.com/armon/go-radix"
)

// rootdata holds static data and constraining rules from the root project for
// use in solving.
type rootdata struct {
	// Path to the root of the project on which gps is operating.
	dir string

	// Map of packages to ignore.
	ig map[string]bool

	// Map of packages to require.
	req map[string]bool

	// A ProjectConstraints map containing the validated (guaranteed non-empty)
	// overrides declared by the root manifest.
	ovr ProjectConstraints

	// A map of the ProjectRoot (local names) that should be allowed to change
	chng map[ProjectRoot]struct{}

	// Flag indicating all projects should be allowed to change, without regard
	// for lock.
	chngall bool

	// A map of the project names listed in the root's lock.
	rlm map[ProjectRoot]LockedProject

	// A defensively-copied instance of the root manifest.
	rm Manifest

	// A defensively-copied instance of the root lock.
	rl Lock

	// A defensively-copied instance of params.RootPackageTree
	rpt PackageTree
}

// rootImportList returns a list of the unique imports from the root data.
// Ignores and requires are taken into consideration.
func (rd rootdata) externalImportList() []string {
	reach := rd.rpt.ExternalReach(true, true, rd.ig).ListExternalImports()

	// If there are any requires, slide them into the reach list, as well.
	if len(rd.req) > 0 {
		reqs := make([]string, 0, len(rd.req))

		// Make a map of both imported and required pkgs to skip, to avoid
		// duplication. Technically, a slice would probably be faster (given
		// small size and bounds check elimination), but this is a one-time op,
		// so it doesn't matter.
		skip := make(map[string]bool, len(rd.req))
		for _, r := range reach {
			if rd.req[r] {
				skip[r] = true
			}
		}

		for r := range rd.req {
			if !skip[r] {
				reqs = append(reqs, r)
			}
		}

		reach = append(reach, reqs...)
	}

	return reach
}

func (rd rootdata) getApplicableConstraints() []workingConstraint {
	xt := radix.New()
	combined := rd.combineConstraints()

	type wccount struct {
		count int
		wc    workingConstraint
	}
	for _, wc := range combined {
		xt.Insert(string(wc.Ident.ProjectRoot), wccount{wc: wc})
	}

	// Walk all dep import paths we have to consider and mark the corresponding
	// wc entry in the trie, if any
	for _, im := range rd.externalImportList() {
		if isStdLib(im) {
			continue
		}

		if pre, v, match := xt.LongestPrefix(im); match && isPathPrefixOrEqual(pre, im) {
			wcc := v.(wccount)
			wcc.count++
			xt.Insert(pre, wcc)
		}
	}

	var ret []workingConstraint

	xt.Walk(func(s string, v interface{}) bool {
		wcc := v.(wccount)
		if wcc.count > 0 || wcc.wc.overrNet || wcc.wc.overrConstraint {
			ret = append(ret, wcc.wc)
		}
		return false
	})

	return ret
}

func (rd rootdata) combineConstraints() []workingConstraint {
	return rd.ovr.overrideAll(rd.rm.DependencyConstraints().merge(rd.rm.TestDependencyConstraints()))
}

// needVersionListFor indicates whether we need a version list for a given
// project root, based solely on general solver inputs (no constraint checking
// required). This will be true if:
//
//  - ChangeAll is on
//  - The project is not in the lock at all
//  - The project is in the lock, but is also in the list of projects to change
func (rd rootdata) needVersionsFor(pr ProjectRoot) bool {
	if rd.chngall {
		return true
	}

	if _, has := rd.rlm[pr]; !has {
		// not in the lock
		return true
	} else if _, has := rd.chng[pr]; has {
		// in the lock, but marked for change
		return true
	}
	// in the lock, not marked for change
	return false

}

func (rd rootdata) isRoot(pr ProjectRoot) bool {
	return pr == ProjectRoot(rd.rpt.ImportRoot)
}

// rootAtom creates an atomWithPackages that represents the root project.
func (rd rootdata) rootAtom() atomWithPackages {
	a := atom{
		id: ProjectIdentifier{
			ProjectRoot: ProjectRoot(rd.rpt.ImportRoot),
		},
		// This is a hack so that the root project doesn't have a nil version.
		// It's sort of OK because the root never makes it out into the results.
		// We may need a more elegant solution if we discover other side
		// effects, though.
		v: rootRev,
	}

	list := make([]string, 0, len(rd.rpt.Packages))
	for path, pkg := range rd.rpt.Packages {
		if pkg.Err != nil && !rd.ig[path] {
			list = append(list, path)
		}
	}
	sort.Strings(list)

	return atomWithPackages{
		a:  a,
		pl: list,
	}
}
