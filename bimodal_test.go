package vsolver

import (
	"fmt"
	"strings"
)

// dsp - "depspec with packages"
//
// Wraps a set of tpkgs onto a depspec, and returns it.
func dsp(ds depspec, pkgs ...tpkg) depspec {
	ds.pkgs = pkgs
	return ds
}

// pkg makes a tpkg appropriate for use in bimodal testing
func pkg(path string, imports ...string) tpkg {
	return tpkg{
		path:    path,
		imports: imports,
	}
}

func init() {
	for k, fix := range bimodalFixtures {
		// Assign the name into the fixture itself
		fix.n = k
		bimodalFixtures[k] = fix
	}
}

// Fixtures that rely on simulated bimodal (project and package-level)
// analysis for correct operation. These all have some extra work done on
// them down in init().
var bimodalFixtures = map[string]bimodalFixture{
	// Simple case, ensures that we do the very basics of picking up and
	// including a single, simple import that is expressed an import
	"simple bm-add": {
		ds: []depspec{
			dsp(dsv("root 0.0.0"),
				pkg("root", "a")),
			dsp(dsv("a 1.0.0"),
				pkg("a")),
		},
		r: mkresults(
			"a 1.0.0",
		),
	},
	// Ensure it works when the import jump is not from the package with the
	// same path as root, but from a subpkg
	"subpkg bm-add": {
		ds: []depspec{
			dsp(dsv("root 0.0.0"),
				pkg("root", "root/foo"),
				pkg("root/foo", "a"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a"),
			),
		},
		r: mkresults(
			"a 1.0.0",
		),
	},
	// Importing package from project with no root package
	"bm-add on project with no pkg in root dir": {
		ds: []depspec{
			dsp(dsv("root 0.0.0"),
				pkg("root", "a/foo")),
			dsp(dsv("a 1.0.0"),
				pkg("a/foo")),
		},
		r: mkresults(
			"a 1.0.0",
		),
	},
	// Import jump is in a dep, and points to a transitive dep
	"transitive bm-add": {
		ds: []depspec{
			dsp(dsv("root 0.0.0"),
				pkg("root", "root/foo"),
				pkg("root/foo", "a"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a", "b"),
			),
			dsp(dsv("b 1.0.0"),
				pkg("b"),
			),
		},
		r: mkresults(
			"a 1.0.0",
			"b 1.0.0",
		),
	},
	// Constraints apply only if the project that declares them has a
	// reachable import
	"constraints activated by import": {
		ds: []depspec{
			dsp(dsv("root 0.0.0", "b 1.0.0"),
				pkg("root", "root/foo"),
				pkg("root/foo", "a"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a", "b"),
			),
			dsp(dsv("b 1.0.0"),
				pkg("b"),
			),
			dsp(dsv("b 1.1.0"),
				pkg("b"),
			),
		},
		r: mkresults(
			"a 1.0.0",
			"b 1.1.0",
		),
	},
	// Import jump is in a dep, and points to a transitive dep - but only in not
	// the first version we try
	"transitive bm-add on older version": {
		ds: []depspec{
			dsp(dsv("root 0.0.0", "a ~1.0.0"),
				pkg("root", "root/foo"),
				pkg("root/foo", "a"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a", "b"),
			),
			dsp(dsv("a 1.1.0"),
				pkg("a"),
			),
			dsp(dsv("b 1.0.0"),
				pkg("b"),
			),
		},
		r: mkresults(
			"a 1.0.0",
			"b 1.0.0",
		),
	},
	// Import jump is in a dep, and points to a transitive dep - but will only
	// get there via backtracking
	"backtrack to dep on bm-add": {
		ds: []depspec{
			dsp(dsv("root 0.0.0"),
				pkg("root", "root/foo"),
				pkg("root/foo", "a", "b"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a", "c"),
			),
			dsp(dsv("a 1.1.0"),
				pkg("a"),
			),
			// Include two versions of b, otherwise it'll be selected first
			dsp(dsv("b 0.9.0"),
				pkg("b", "c"),
			),
			dsp(dsv("b 1.0.0"),
				pkg("b", "c"),
			),
			dsp(dsv("c 1.0.0", "a 1.0.0"),
				pkg("c", "a"),
			),
		},
		r: mkresults(
			"a 1.0.0",
			"b 1.0.0",
			"c 1.0.0",
		),
	},
	// Import jump is in a dep subpkg, and points to a transitive dep
	"transitive subpkg bm-add": {
		ds: []depspec{
			dsp(dsv("root 0.0.0"),
				pkg("root", "root/foo"),
				pkg("root/foo", "a"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a", "a/bar"),
				pkg("a/bar", "b"),
			),
			dsp(dsv("b 1.0.0"),
				pkg("b"),
			),
		},
		r: mkresults(
			"a 1.0.0",
			"b 1.0.0",
		),
	},
	// Import jump is in a dep subpkg, pointing to a transitive dep, but only in
	// not the first version we try
	"transitive subpkg bm-add on older version": {
		ds: []depspec{
			dsp(dsv("root 0.0.0", "a ~1.0.0"),
				pkg("root", "root/foo"),
				pkg("root/foo", "a"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a", "a/bar"),
				pkg("a/bar", "b"),
			),
			dsp(dsv("a 1.1.0"),
				pkg("a", "a/bar"),
			),
			dsp(dsv("b 1.0.0"),
				pkg("b"),
			),
		},
		r: mkresults(
			"a 1.0.0",
			"b 1.0.0",
		),
	},
	// Ensure that if a constraint is expressed, but no actual import exists,
	// then the constraint is disregarded - the project named in the constraint
	// is not part of the solution.
	"ignore constraint without import": {
		ds: []depspec{
			dsp(dsv("root 0.0.0", "a 1.0.0"),
				pkg("root", "root/foo"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a"),
			),
		},
		r: mkresults(),
	},
	// Transitive deps from one project (a) get incrementally included as other
	// deps incorporate its various packages.
	"multi-stage pkg incorporation": {
		ds: []depspec{
			dsp(dsv("root 0.0.0"),
				pkg("root", "a", "d"),
			),
			dsp(dsv("a 1.0.0"),
				pkg("a", "b"),
				pkg("a/second", "c"),
			),
			dsp(dsv("b 2.0.0"),
				pkg("b"),
			),
			dsp(dsv("c 1.2.0"),
				pkg("c"),
			),
			dsp(dsv("d 1.0.0"),
				pkg("d", "a/second"),
			),
		},
		r: mkresults(
			"a 1.0.0",
			"b 2.0.0",
			"c 1.2.0",
			"d 1.0.0",
		),
	},
}

// tpkg is a representation of a single package. It has its own import path, as
// well as a list of paths it itself "imports".
type tpkg struct {
	// Full import path of this package
	path string
	// Slice of full paths to its virtual imports
	imports []string
}

type bimodalFixture struct {
	// name of this fixture datum
	n string
	// bimodal project. first is always treated as root project
	ds []depspec
	// results; map of name/version pairs
	r map[string]Version
	// max attempts the solver should need to find solution. 0 means no limit
	maxAttempts int
	// Use downgrade instead of default upgrade sorter
	downgrade bool
	// lock file simulator, if one's to be used at all
	l fixLock
	// projects expected to have errors, if any
	errp []string
	// request up/downgrade to all projects
	changeall bool
}

func (f bimodalFixture) name() string {
	return f.n
}

func (f bimodalFixture) specs() []depspec {
	return f.ds
}

func (f bimodalFixture) maxTries() int {
	return f.maxAttempts
}

func (f bimodalFixture) expectErrs() []string {
	return f.errp
}

func (f bimodalFixture) result() map[string]Version {
	return f.r
}

// bmSourceManager is an SM specifically for the bimodal fixtures. It composes
// the general depspec SM, and differs from it only in how it answers
// ExternalReach() calls.
type bmSourceManager struct {
	depspecSourceManager
}

var _ SourceManager = &bmSourceManager{}

func newbmSM(ds []depspec) *bmSourceManager {
	sm := &bmSourceManager{}
	sm.specs = ds
	sm.rm = computeBimodalExternalMap(ds)

	return sm
}

func (sm *bmSourceManager) ListPackages(n ProjectName, v Version) (map[string]string, error) {
	for k, ds := range sm.specs {
		// Cheat for root, otherwise we blow up b/c version is empty
		if n == ds.n && (k == 0 || ds.v.Matches(v)) {
			m := make(map[string]string)

			for _, pkg := range ds.pkgs {
				m[pkg.path] = pkg.path
			}

			return m, nil
		}
	}

	return nil, fmt.Errorf("Project %s at version %s could not be found", n, v)
}

func (sm *bmSourceManager) ExternalReach(n ProjectName, v Version) (map[string][]string, error) {
	for _, ds := range sm.specs {
		if ds.n == n && v.Matches(ds.v) {
			rm := make(map[string][]string)
			for _, pkg := range ds.pkgs {
				rm[pkg.path] = pkg.imports
			}

			return rm, nil
		}
	}

	// TODO proper solver errs
	return nil, fmt.Errorf("No reach data for %s at version %s", n, v)
}

// computeBimodalExternalMap takes a set of depspecs and computes an
// internally-versioned external reach map that is useful for quickly answering
// ListExternal()-type calls.
//
// Note that it does not do things like stripping out stdlib packages - these
// maps are intended for use in SM fixtures, and that's a higher-level
// responsibility within the system.
func computeBimodalExternalMap(ds []depspec) map[pident][]string {
	rm := make(map[pident][]string)

	for _, d := range ds {
		exmap := make(map[string]struct{})

		for _, pkg := range d.pkgs {
			for _, ex := range pkg.imports {
				if !strings.HasPrefix(ex, string(d.n)) {
					exmap[ex] = struct{}{}
				}
			}
		}

		var list []string
		for ex := range exmap {
			list = append(list, ex)
		}
		id := pident{
			n: d.n,
			v: d.v,
		}
		rm[id] = list
	}

	return rm
}
