package vsolver

import (
	"fmt"

	"github.com/armon/go-radix"
)

// mkbmu - "make bimodal universe"
//
// Assembles a universe of projects and packages - that is, a slice of depspecs -
// from discrete project and package declarations.
//
// Projects must be declared before any containing packages, or this function
// will panic. Projects cannot be declared more than once (else panic). Projects
// cannot be nested. A project does not imply an importable package; both must
// be explicitly declared.
func mkbmu(list ...bmelem) (ret []depspec) {
	xt := radix.New()

	var rootname string
	for k, elem := range list {
		switch p := elem.(type) {
		case depspec:
			if k == 0 {
				rootname = string(p.Name())
			}
			xt.WalkPath(string(p.Name()), func(s string, v interface{}) bool {
				panic(fmt.Sprintf("Got bmproj with name %s, but already had %s. Do not duplicate or declare projects relative to each other.", p.Name(), s))
			})

			xt.Insert(string(p.Name()), p)
		case tpkg:
			var success bool
			xt.WalkPath(p.path, func(s string, v interface{}) bool {
				if proj, ok := v.(depspec); ok {
					proj.pkgs = append(proj.pkgs, p)
					_, ok = xt.Insert(string(proj.Name()), proj)
					success = true
					//if !ok {
					//panic(fmt.Sprintf("Failed to reinsert updated bmproj %s", proj.Name()))
					//}
				}
				return false
			})
			if !success {
				panic(fmt.Sprintf("Couldn't find parent project for %s. mkbmu is sensitive to parameter order; always declare the root project first.", p.path))
			}
		default:
			panic(fmt.Sprintf("Unrecognized bmelem type %T", elem))
		}
	}

	// Ensure root always goes in first
	val, _ := xt.Get(rootname)
	ret = append(ret, val.(depspec))
	for _, pi := range xt.ToMap() {
		if p, ok := pi.(depspec); ok {
			ret = append(ret, p)
		}
	}

	return
}

// mkpkr - "make package" - makes a tpkg appropriate for use in bimodal testing
func mkpkg(path string, imports ...string) tpkg {
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
	"simple bimodal add": {
		ds: mkbmu(
			dsv("root 0.0.0"),
			mkpkg("root", "a"),
			dsv("a 1.0.0"),
			mkpkg("a"),
		),
		r: mkresults(
			"a 1.0.0",
		),
	},
	// Ensure it works when the import jump is not from the package with the
	// same path as root, but from a subpkg
	"subpkg bimodal add": {
		ds: mkbmu(
			dsv("root 0.0.0"),
			mkpkg("root", "root/foo"),
			mkpkg("root/foo", "a"),
			dsv("a 1.0.0"),
			mkpkg("a"),
		),
		r: mkresults(
			"a 1.0.0",
		),
	},
	// Ensure that if a constraint is expressed, but no actual import exists,
	// then the constraint is disregarded - the project named in the constraint
	// is not part of the solution.
	"ignore constraint without import": {
		ds: mkbmu(
			dsv("root 0.0.0", "a 1.0.0"),
			mkpkg("root", "root/foo"),
			dsv("a 1.0.0"),
			mkpkg("a"),
		),
		r: mkresults(),
	},
}

//type bmproj struct {
//n       ProjectName
//v       Version
//deps    []ProjectDep
//devdeps []ProjectDep
//pkgs    []tpkg
//}

//var _ Manifest = bmproj{}

//// impl Spec interface
//func (p bmproj) GetDependencies() []ProjectDep {
//return p.deps
//}

//// impl Spec interface
//func (p bmproj) GetDevDependencies() []ProjectDep {
//return p.devdeps
//}

//// impl Spec interface
//func (p bmproj) Name() ProjectName {
//return p.n
//}

type tpkg struct {
	// Full import path of this package
	path string
	// Slice of full paths to its virtual imports
	imports []string
}

type bmelem interface {
	_bmelem()
}

func (p tpkg) _bmelem() {}

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

func computeBimodalExternalMap(ds []depspec) map[pident][]string {
	rm := make(map[pident][]string)

	for _, d := range ds {
		exmap := make(map[string]struct{})

		for _, pkg := range d.pkgs {
			for _, ex := range pkg.imports {
				exmap[ex] = struct{}{}
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
