package vsolver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
)

// nsvSplit splits an "info" string on " " into the pair of name and
// version/constraint, and returns each individually.
//
// This is for narrow use - panics if there are less than two resulting items in
// the slice.
func nsvSplit(info string) (name string, version string) {
	s := strings.SplitN(info, " ", 2)
	if len(s) < 2 {
		panic(fmt.Sprintf("Malformed name/version info string '%s'", info))
	}

	name, version = s[0], s[1]
	return
}

// mksvpa - "make semver project atom"
//
// Splits the input string on a space, and uses the first two elements as the
// project name and constraint body, respectively.
func mksvpa(info string) ProjectAtom {
	name, v := nsvSplit(info)

	sv, err := semver.NewVersion(v)
	if err != nil {
		// don't want to allow bad test data at this level, so just panic
		panic(fmt.Sprintf("Error when converting '%s' into semver: %s", v, err))
	}

	return ProjectAtom{
		Name: ProjectName(name),
		Version: Version{
			Type:   V_Semver,
			Info:   v,
			SemVer: sv,
		},
	}
}

// mkc - "make constraint"
func mkc(body string, t ConstraintType) Constraint {
	c, err := NewConstraint(t, body)
	if err != nil {
		// don't want bad test data at this level, so just panic
		panic(fmt.Sprintf("Error when converting '%s' into semver constraint: %s", body, err))
	}

	return c
}

// mksvd - "make semver dependency"
//
// Splits the input string on a space, and uses the first two elements as the
// project name and constraint body, respectively.
func mksvd(info string) ProjectDep {
	name, v := nsvSplit(info)

	return ProjectDep{
		Name:       ProjectName(name),
		Constraint: mkc(v, C_Semver),
	}
}

type depspec struct {
	name    ProjectAtom
	deps    []ProjectDep
	devdeps []ProjectDep
}

// dsv - "depspec semver" (make a semver depspec)
//
// Wraps up all the other semver-making-helper funcs to create a depspec with
// both semver versions and constraints.
//
// As it assembles from the other shortcut methods, it'll panic if anything's
// malformed.
//
// First string is broken out into the name/semver of the main package.
func dsv(pi string, deps ...string) depspec {
	ds := depspec{
		name: mksvpa(pi),
	}

	for _, dep := range deps {
		if strings.HasPrefix(dep, "(dev) ") {
			ds.devdeps = append(ds.devdeps, mksvd(strings.TrimPrefix(dep, "(dev) ")))
		} else {
			ds.deps = append(ds.deps, mksvd(dep))
		}
	}

	return ds
}

type fixture struct {
	// name of this fixture datum
	n string
	// depspecs. always treat first as root
	ds []depspec
	// results; map of name/version pairs
	r map[string]string
	// max attempts the solver should need to find solution. 0 means no limit
	maxAttempts int
	// Use downgrade instead of default upgrade sorter
	downgrade bool
	// lock file simulator, if one's to be used at all
	l Lock
	// projects expected to have errors, if any
	errp []string
}

// mklock makes a fixLock, suitable to act as a lock file
func mklock(pairs ...string) fixLock {
	l := make(fixLock)
	for _, s := range pairs {
		pa := mksvpa(s)
		l[pa.Name] = pa
	}

	return l
}

// mkresults makes a result set
func mkresults(pairs ...string) map[string]string {
	m := make(map[string]string)
	for _, pair := range pairs {
		name, v := nsvSplit(pair)
		m[name] = v
	}

	return m
}

var fixtures = []fixture{
	// basic fixtures
	{
		n: "no dependencies",
		ds: []depspec{
			dsv("root 0.0.0"),
		},
		r: mkresults("root 0.0.0"),
	},
	{
		n: "simple dependency tree",
		ds: []depspec{
			dsv("root 0.0.0", "a 1.0.0", "b 1.0.0"),
			dsv("a 1.0.0", "aa 1.0.0", "ab 1.0.0"),
			dsv("aa 1.0.0"),
			dsv("ab 1.0.0"),
			dsv("b 1.0.0", "ba 1.0.0", "bb 1.0.0"),
			dsv("ba 1.0.0"),
			dsv("bb 1.0.0"),
		},
		r: mkresults(
			"root 0.0.0",
			"a 1.0.0",
			"aa 1.0.0",
			"ab 1.0.0",
			"b 1.0.0",
			"ba 1.0.0",
			"bb 1.0.0",
		),
	},
	{
		n: "shared dependency with overlapping constraints",
		ds: []depspec{
			dsv("root 0.0.0", "a 1.0.0", "b 1.0.0"),
			dsv("a 1.0.0", "shared >=2.0.0, <4.0.0"),
			dsv("b 1.0.0", "shared >=3.0.0, <5.0.0"),
			dsv("shared 2.0.0"),
			dsv("shared 3.0.0"),
			dsv("shared 3.6.9"),
			dsv("shared 4.0.0"),
			dsv("shared 5.0.0"),
		},
		r: mkresults(
			"root 0.0.0",
			"a 1.0.0",
			"b 1.0.0",
			"shared 3.6.9",
		),
	},
	{
		n: "shared dependency where dependent version in turn affects other dependencies",
		ds: []depspec{
			dsv("root 0.0.0", "foo <=1.0.2", "bar 1.0.0"),
			dsv("foo 1.0.0"),
			dsv("foo 1.0.1", "bang 1.0.0"),
			dsv("foo 1.0.2", "whoop 1.0.0"),
			dsv("foo 1.0.3", "zoop 1.0.0"),
			dsv("bar 1.0.0", "foo <=1.0.1"),
			dsv("bang 1.0.0"),
			dsv("whoop 1.0.0"),
			dsv("zoop 1.0.0"),
		},
		r: mkresults(
			"root 0.0.0",
			"foo 1.0.1",
			"bar 1.0.0",
			"bang 1.0.0",
		),
	},
	{
		n: "removed dependency",
		ds: []depspec{
			dsv("root 1.0.0", "foo 1.0.0", "bar *"),
			dsv("foo 1.0.0"),
			dsv("foo 2.0.0"),
			dsv("bar 1.0.0"),
			dsv("bar 2.0.0", "baz 1.0.0"),
			dsv("baz 1.0.0", "foo 1.0.0"),
		},
		r: mkresults(
			"root 1.0.0",
			"foo 1.0.0",
			"bar 1.0.0",
		),
		maxAttempts: 2,
	},
	// fixtures with locks
	{
		n: "with compatible locked dependency",
		ds: []depspec{
			dsv("root 0.0.0", "foo *"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.1", "bar 1.0.1"),
			dsv("foo 1.0.2", "bar 1.0.2"),
			dsv("bar 1.0.0"),
			dsv("bar 1.0.1"),
			dsv("bar 1.0.2"),
		},
		l: mklock(
			"foo 1.0.1",
		),
		r: mkresults(
			"root 0.0.0",
			"foo 1.0.1",
			"bar 1.0.1",
		),
	},
	{
		n: "with incompatible locked dependency",
		ds: []depspec{
			dsv("root 0.0.0", "foo >1.0.1"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.1", "bar 1.0.1"),
			dsv("foo 1.0.2", "bar 1.0.2"),
			dsv("bar 1.0.0"),
			dsv("bar 1.0.1"),
			dsv("bar 1.0.2"),
		},
		l: mklock(
			"foo 1.0.1",
		),
		r: mkresults(
			"root 0.0.0",
			"foo 1.0.2",
			"bar 1.0.2",
		),
	},
	{
		n: "with unrelated locked dependency",
		ds: []depspec{
			dsv("root 0.0.0", "foo *"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.1", "bar 1.0.1"),
			dsv("foo 1.0.2", "bar 1.0.2"),
			dsv("bar 1.0.0"),
			dsv("bar 1.0.1"),
			dsv("bar 1.0.2"),
			dsv("baz 1.0.0"),
		},
		l: mklock(
			"baz 1.0.0",
		),
		r: mkresults(
			"root 0.0.0",
			"foo 1.0.2",
			"bar 1.0.2",
		),
	},
	{
		n: "unlocks dependencies if necessary to ensure that a new dependency is satisfied",
		ds: []depspec{
			dsv("root 0.0.0", "foo *", "newdep *"),
			dsv("foo 1.0.0", "bar <2.0.0"),
			dsv("bar 1.0.0", "baz <2.0.0"),
			dsv("baz 1.0.0", "qux <2.0.0"),
			dsv("qux 1.0.0"),
			dsv("foo 2.0.0", "bar <3.0.0"),
			dsv("bar 2.0.0", "baz <3.0.0"),
			dsv("baz 2.0.0", "qux <3.0.0"),
			dsv("qux 2.0.0"),
			dsv("newdep 2.0.0", "baz >=1.5.0"),
		},
		l: mklock(
			"foo 1.0.0",
			"bar 1.0.0",
			"baz 1.0.0",
			"qux 1.0.0",
		),
		r: mkresults(
			"root 0.0.0",
			"foo 2.0.0",
			"bar 2.0.0",
			"baz 2.0.0",
			"qux 1.0.0",
			"newdep 2.0.0",
		),
		maxAttempts: 4,
	},
	{
		n: "includes root package's dev dependencies",
		ds: []depspec{
			dsv("root 1.0.0", "(dev) foo 1.0.0", "(dev) bar 1.0.0"),
			dsv("foo 1.0.0"),
			dsv("bar 1.0.0"),
		},
		r: mkresults(
			"root 1.0.0",
			"foo 1.0.0",
			"bar 1.0.0",
		),
	},
	{
		n: "includes dev dependency's transitive dependencies",
		ds: []depspec{
			dsv("root 1.0.0", "(dev) foo 1.0.0"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("bar 1.0.0"),
		},
		r: mkresults(
			"root 1.0.0",
			"foo 1.0.0",
			"bar 1.0.0",
		),
	},
	{
		n: "ignores transitive dependency's dev dependencies",
		ds: []depspec{
			dsv("root 1.0.0", "(dev) foo 1.0.0"),
			dsv("foo 1.0.0", "(dev) bar 1.0.0"),
			dsv("bar 1.0.0"),
		},
		r: mkresults(
			"root 1.0.0",
			"foo 1.0.0",
		),
	},
	{
		n: "no version that matches requirement",
		ds: []depspec{
			dsv("root 0.0.0", "foo >=1.0.0, <2.0.0"),
			dsv("foo 2.0.0"),
			dsv("foo 2.1.3"),
		},
		errp: []string{"foo", "root"},
	},
}

type depspecSourceManager struct {
	specs []depspec
	//map[ProjectAtom][]Version
	sortup bool
}

var _ SourceManager = &depspecSourceManager{}

func newdepspecSM(ds []depspec, upgrade bool) *depspecSourceManager {
	//TODO precompute the version lists, for speediness?
	return &depspecSourceManager{
		specs:  ds,
		sortup: upgrade,
	}
}

func (sm *depspecSourceManager) GetProjectInfo(pa ProjectAtom) (ProjectInfo, error) {
	for _, ds := range sm.specs {
		if pa.Name == ds.name.Name && pa.Version.Info == ds.name.Version.Info {
			return ProjectInfo{
				pa:       ds.name,
				Manifest: ds,
				Lock:     dummyLock{},
			}, nil
		}
	}

	// TODO proper solver-type errors
	return ProjectInfo{}, fmt.Errorf("Project '%s' at version '%s' could not be found", pa.Name, pa.Version.Info)
}

func (sm *depspecSourceManager) ListVersions(name ProjectName) (pi []Version, err error) {
	for _, ds := range sm.specs {
		if name == ds.name.Name {
			pi = append(pi, ds.name.Version)
		}
	}

	if len(pi) == 0 {
		err = fmt.Errorf("Project '%s' could not be found", name)
	}

	if sm.sortup {
		sort.Sort(upgradeVersionSorter(pi))
	} else {
		sort.Sort(downgradeVersionSorter(pi))
	}

	return
}

func (sm *depspecSourceManager) ProjectExists(name ProjectName) bool {
	for _, ds := range sm.specs {
		if name == ds.name.Name {
			return true
		}
	}

	return false
}

// enforce interfaces
var _ Manifest = depspec{}
var _ Lock = dummyLock{}
var _ Lock = fixLock{}

// impl Spec interface
func (ds depspec) GetDependencies() []ProjectDep {
	return ds.deps
}

// impl Spec interface
func (ds depspec) GetDevDependencies() []ProjectDep {
	return ds.devdeps
}

// impl Spec interface
func (ds depspec) Name() ProjectName {
	return ds.name.Name
}

type fixLock map[ProjectName]ProjectAtom

func (fixLock) SolverVersion() string {
	return "-1"
}

// impl Lock interface
func (fixLock) InputHash() string {
	return "fooooorooooofooorooofoo"
}

// impl Lock interface
func (l fixLock) GetProjectAtom(n ProjectName) *ProjectAtom {
	if pa, exists := l[n]; exists {
		return &pa
	}
	return nil
}

type dummyLock struct{}

// impl Lock interface
func (_ dummyLock) SolverVersion() string {
	return "-1"
}

// impl Lock interface
func (_ dummyLock) InputHash() string {
	return "fooooorooooofooorooofoo"
}

// impl Lock interface
func (_ dummyLock) GetProjectAtom(_ ProjectName) *ProjectAtom {
	return nil
}

// We've borrowed this bestiary from pub's tests:
// https://github.com/dart-lang/pub/blob/master/test/version_solver_test.dart

// TODO finish converting all of these
// TODO ...figure out project-vs-pkg thing so we even know if these are useful

/*
func basicGraph() {
  testResolve("circular dependency", {
    "myapp 1.0.0": {
      "foo": "1.0.0"
    },
    "foo 1.0.0": {
      "bar": "1.0.0"
    },
    "bar 1.0.0": {
      "foo": "1.0.0"
    }
  }, result: {
    "myapp from root": "1.0.0",
    "foo": "1.0.0",
    "bar": "1.0.0"
  });

}

func withLockFile() {

}

func rootDependency() {
  testResolve("with root source", {
    "myapp 1.0.0": {
      "foo": "1.0.0"
    },
    "foo 1.0.0": {
      "myapp from root": ">=1.0.0"
    }
  }, result: {
    "myapp from root": "1.0.0",
    "foo": "1.0.0"
  });

  testResolve("with different source", {
    "myapp 1.0.0": {
      "foo": "1.0.0"
    },
    "foo 1.0.0": {
      "myapp": ">=1.0.0"
    }
  }, result: {
    "myapp from root": "1.0.0",
    "foo": "1.0.0"
  });

  testResolve("with mismatched sources", {
    "myapp 1.0.0": {
      "foo": "1.0.0",
      "bar": "1.0.0"
    },
    "foo 1.0.0": {
      "myapp": ">=1.0.0"
    },
    "bar 1.0.0": {
      "myapp from mock2": ">=1.0.0"
    }
  }, error: sourceMismatch("myapp", "foo", "bar"));

  testResolve("with wrong version", {
    "myapp 1.0.0": {
      "foo": "1.0.0"
    },
    "foo 1.0.0": {
      "myapp": "<1.0.0"
    }
  }, error: couldNotSolve);
}

func unsolvable() {
  testResolve("no version that matches combined constraint", {
    "myapp 0.0.0": {
      "foo": "1.0.0",
      "bar": "1.0.0"
    },
    "foo 1.0.0": {
      "shared": ">=2.0.0 <3.0.0"
    },
    "bar 1.0.0": {
      "shared": ">=2.9.0 <4.0.0"
    },
    "shared 2.5.0": {},
    "shared 3.5.0": {}
  }, error: noVersion(["shared", "foo", "bar"]));

  testResolve("disjoint constraints", {
    "myapp 0.0.0": {
      "foo": "1.0.0",
      "bar": "1.0.0"
    },
    "foo 1.0.0": {
      "shared": "<=2.0.0"
    },
    "bar 1.0.0": {
      "shared": ">3.0.0"
    },
    "shared 2.0.0": {},
    "shared 4.0.0": {}
  }, error: disjointConstraint(["shared", "foo", "bar"]));

  testResolve("mismatched descriptions", {
    "myapp 0.0.0": {
      "foo": "1.0.0",
      "bar": "1.0.0"
    },
    "foo 1.0.0": {
      "shared-x": "1.0.0"
    },
    "bar 1.0.0": {
      "shared-y": "1.0.0"
    },
    "shared-x 1.0.0": {},
    "shared-y 1.0.0": {}
  }, error: descriptionMismatch("shared", "foo", "bar"));

  testResolve("mismatched sources", {
    "myapp 0.0.0": {
      "foo": "1.0.0",
      "bar": "1.0.0"
    },
    "foo 1.0.0": {
      "shared": "1.0.0"
    },
    "bar 1.0.0": {
      "shared from mock2": "1.0.0"
    },
    "shared 1.0.0": {},
    "shared 1.0.0 from mock2": {}
  }, error: sourceMismatch("shared", "foo", "bar"));

  testResolve("no valid solution", {
    "myapp 0.0.0": {
      "a": "any",
      "b": "any"
    },
    "a 1.0.0": {
      "b": "1.0.0"
    },
    "a 2.0.0": {
      "b": "2.0.0"
    },
    "b 1.0.0": {
      "a": "2.0.0"
    },
    "b 2.0.0": {
      "a": "1.0.0"
    }
  }, error: couldNotSolve, maxTries: 2);

  // This is a regression test for #15550.
  testResolve("no version that matches while backtracking", {
    "myapp 0.0.0": {
      "a": "any",
      "b": ">1.0.0"
    },
    "a 1.0.0": {},
    "b 1.0.0": {}
  }, error: noVersion(["myapp", "b"]), maxTries: 1);


  // This is a regression test for #18300.
  testResolve("...", {
    "myapp 0.0.0": {
      "angular": "any",
      "collection": "any"
    },
    "analyzer 0.12.2": {},
    "angular 0.10.0": {
      "di": ">=0.0.32 <0.1.0",
      "collection": ">=0.9.1 <1.0.0"
    },
    "angular 0.9.11": {
      "di": ">=0.0.32 <0.1.0",
      "collection": ">=0.9.1 <1.0.0"
    },
    "angular 0.9.10": {
      "di": ">=0.0.32 <0.1.0",
      "collection": ">=0.9.1 <1.0.0"
    },
    "collection 0.9.0": {},
    "collection 0.9.1": {},
    "di 0.0.37": {"analyzer": ">=0.13.0 <0.14.0"},
    "di 0.0.36": {"analyzer": ">=0.13.0 <0.14.0"}
  }, error: noVersion(["analyzer", "di"]), maxTries: 2);
}

func badSource() {
  testResolve("fail if the root package has a bad source in dep", {
    "myapp 0.0.0": {
      "foo from bad": "any"
    },
  }, error: unknownSource("myapp", "foo", "bad"));

  testResolve("fail if the root package has a bad source in dev dep", {
    "myapp 0.0.0": {
      "(dev) foo from bad": "any"
    },
  }, error: unknownSource("myapp", "foo", "bad"));

  testResolve("fail if all versions have bad source in dep", {
    "myapp 0.0.0": {
      "foo": "any"
    },
    "foo 1.0.0": {
      "bar from bad": "any"
    },
    "foo 1.0.1": {
      "baz from bad": "any"
    },
    "foo 1.0.3": {
      "bang from bad": "any"
    },
  }, error: unknownSource("foo", "bar", "bad"), maxTries: 3);

  testResolve("ignore versions with bad source in dep", {
    "myapp 1.0.0": {
      "foo": "any"
    },
    "foo 1.0.0": {
      "bar": "any"
    },
    "foo 1.0.1": {
      "bar from bad": "any"
    },
    "foo 1.0.3": {
      "bar from bad": "any"
    },
    "bar 1.0.0": {}
  }, result: {
    "myapp from root": "1.0.0",
    "foo": "1.0.0",
    "bar": "1.0.0"
  }, maxTries: 3);
}

func backtracking() {
  testResolve("circular dependency on older version", {
    "myapp 0.0.0": {
      "a": ">=1.0.0"
    },
    "a 1.0.0": {},
    "a 2.0.0": {
      "b": "1.0.0"
    },
    "b 1.0.0": {
      "a": "1.0.0"
    }
  }, result: {
    "myapp from root": "0.0.0",
    "a": "1.0.0"
  }, maxTries: 2);

  // The latest versions of a and b disagree on c. An older version of either
  // will resolve the problem. This test validates that b, which is farther
  // in the dependency graph from myapp is downgraded first.
  testResolve("rolls back leaf versions first", {
    "myapp 0.0.0": {
      "a": "any"
    },
    "a 1.0.0": {
      "b": "any"
    },
    "a 2.0.0": {
      "b": "any",
      "c": "2.0.0"
    },
    "b 1.0.0": {},
    "b 2.0.0": {
      "c": "1.0.0"
    },
    "c 1.0.0": {},
    "c 2.0.0": {}
  }, result: {
    "myapp from root": "0.0.0",
    "a": "2.0.0",
    "b": "1.0.0",
    "c": "2.0.0"
  }, maxTries: 2);

  // Only one version of baz, so foo and bar will have to downgrade until they
  // reach it.
  testResolve("simple transitive", {
    "myapp 0.0.0": {"foo": "any"},
    "foo 1.0.0": {"bar": "1.0.0"},
    "foo 2.0.0": {"bar": "2.0.0"},
    "foo 3.0.0": {"bar": "3.0.0"},
    "bar 1.0.0": {"baz": "any"},
    "bar 2.0.0": {"baz": "2.0.0"},
    "bar 3.0.0": {"baz": "3.0.0"},
    "baz 1.0.0": {}
  }, result: {
    "myapp from root": "0.0.0",
    "foo": "1.0.0",
    "bar": "1.0.0",
    "baz": "1.0.0"
  }, maxTries: 3);

  // This ensures it doesn"t exhaustively search all versions of b when it"s
  // a-2.0.0 whose dependency on c-2.0.0-nonexistent led to the problem. We
  // make sure b has more versions than a so that the solver tries a first
  // since it sorts sibling dependencies by number of versions.
  testResolve("backjump to nearer unsatisfied package", {
    "myapp 0.0.0": {
      "a": "any",
      "b": "any"
    },
    "a 1.0.0": { "c": "1.0.0" },
    "a 2.0.0": { "c": "2.0.0-nonexistent" },
    "b 1.0.0": {},
    "b 2.0.0": {},
    "b 3.0.0": {},
    "c 1.0.0": {},
  }, result: {
    "myapp from root": "0.0.0",
    "a": "1.0.0",
    "b": "3.0.0",
    "c": "1.0.0"
  }, maxTries: 2);

  // Tests that the backjumper will jump past unrelated selections when a
  // source conflict occurs. This test selects, in order:
  // - myapp -> a
  // - myapp -> b
  // - myapp -> c (1 of 5)
  // - b -> a
  // It selects a and b first because they have fewer versions than c. It
  // traverses b"s dependency on a after selecting a version of c because
  // dependencies are traversed breadth-first (all of myapps"s immediate deps
  // before any other their deps).
  //
  // This means it doesn"t discover the source conflict until after selecting
  // c. When that happens, it should backjump past c instead of trying older
  // versions of it since they aren"t related to the conflict.
  testResolve("backjump to conflicting source", {
    "myapp 0.0.0": {
      "a": "any",
      "b": "any",
      "c": "any"
    },
    "a 1.0.0": {},
    "a 1.0.0 from mock2": {},
    "b 1.0.0": {
      "a": "any"
    },
    "b 2.0.0": {
      "a from mock2": "any"
    },
    "c 1.0.0": {},
    "c 2.0.0": {},
    "c 3.0.0": {},
    "c 4.0.0": {},
    "c 5.0.0": {},
  }, result: {
    "myapp from root": "0.0.0",
    "a": "1.0.0",
    "b": "1.0.0",
    "c": "5.0.0"
  }, maxTries: 2);

  // Like the above test, but for a conflicting description.
  testResolve("backjump to conflicting description", {
    "myapp 0.0.0": {
      "a-x": "any",
      "b": "any",
      "c": "any"
    },
    "a-x 1.0.0": {},
    "a-y 1.0.0": {},
    "b 1.0.0": {
      "a-x": "any"
    },
    "b 2.0.0": {
      "a-y": "any"
    },
    "c 1.0.0": {},
    "c 2.0.0": {},
    "c 3.0.0": {},
    "c 4.0.0": {},
    "c 5.0.0": {},
  }, result: {
    "myapp from root": "0.0.0",
    "a": "1.0.0",
    "b": "1.0.0",
    "c": "5.0.0"
  }, maxTries: 2);

  // Similar to the above two tests but where there is no solution. It should
  // fail in this case with no backtracking.
  testResolve("backjump to conflicting source", {
    "myapp 0.0.0": {
      "a": "any",
      "b": "any",
      "c": "any"
    },
    "a 1.0.0": {},
    "a 1.0.0 from mock2": {},
    "b 1.0.0": {
      "a from mock2": "any"
    },
    "c 1.0.0": {},
    "c 2.0.0": {},
    "c 3.0.0": {},
    "c 4.0.0": {},
    "c 5.0.0": {},
  }, error: sourceMismatch("a", "myapp", "b"), maxTries: 1);

  testResolve("backjump to conflicting description", {
    "myapp 0.0.0": {
      "a-x": "any",
      "b": "any",
      "c": "any"
    },
    "a-x 1.0.0": {},
    "a-y 1.0.0": {},
    "b 1.0.0": {
      "a-y": "any"
    },
    "c 1.0.0": {},
    "c 2.0.0": {},
    "c 3.0.0": {},
    "c 4.0.0": {},
    "c 5.0.0": {},
  }, error: descriptionMismatch("a", "myapp", "b"), maxTries: 1);

  // Dependencies are ordered so that packages with fewer versions are tried
  // first. Here, there are two valid solutions (either a or b must be
  // downgraded once). The chosen one depends on which dep is traversed first.
  // Since b has fewer versions, it will be traversed first, which means a will
  // come later. Since later selections are revised first, a gets downgraded.
  testResolve("traverse into package with fewer versions first", {
    "myapp 0.0.0": {
      "a": "any",
      "b": "any"
    },
    "a 1.0.0": {"c": "any"},
    "a 2.0.0": {"c": "any"},
    "a 3.0.0": {"c": "any"},
    "a 4.0.0": {"c": "any"},
    "a 5.0.0": {"c": "1.0.0"},
    "b 1.0.0": {"c": "any"},
    "b 2.0.0": {"c": "any"},
    "b 3.0.0": {"c": "any"},
    "b 4.0.0": {"c": "2.0.0"},
    "c 1.0.0": {},
    "c 2.0.0": {},
  }, result: {
    "myapp from root": "0.0.0",
    "a": "4.0.0",
    "b": "4.0.0",
    "c": "2.0.0"
  }, maxTries: 2);

  // This is similar to the above test. When getting the number of versions of
  // a package to determine which to traverse first, versions that are
  // disallowed by the root package"s constraints should not be considered.
  // Here, foo has more versions of bar in total (4), but fewer that meet
  // myapp"s constraints (only 2). There is no solution, but we will do less
  // backtracking if foo is tested first.
  testResolve("take root package constraints into counting versions", {
    "myapp 0.0.0": {
      "foo": ">2.0.0",
      "bar": "any"
    },
    "foo 1.0.0": {"none": "2.0.0"},
    "foo 2.0.0": {"none": "2.0.0"},
    "foo 3.0.0": {"none": "2.0.0"},
    "foo 4.0.0": {"none": "2.0.0"},
    "bar 1.0.0": {},
    "bar 2.0.0": {},
    "bar 3.0.0": {},
    "none 1.0.0": {}
  }, error: noVersion(["foo", "none"]), maxTries: 2);

  // This sets up a hundred versions of foo and bar, 0.0.0 through 9.9.0. Each
  // version of foo depends on a baz with the same major version. Each version
  // of bar depends on a baz with the same minor version. There is only one
  // version of baz, 0.0.0, so only older versions of foo and bar will
  // satisfy it.
  var mapp = {
    "myapp 0.0.0": {
      "foo": "any",
      "bar": "any"
    },
    "baz 0.0.0": {}
  };

  for (var i = 0; i < 10; i++) {
    for (var j = 0; j < 10; j++) {
      mapp["foo $i.$j.0"] = {"baz": "$i.0.0"};
      mapp["bar $i.$j.0"] = {"baz": "0.$j.0"};
    }
  }

  testResolve("complex backtrack", map, result: {
    "myapp from root": "0.0.0",
    "foo": "0.9.0",
    "bar": "9.0.0",
    "baz": "0.0.0"
  }, maxTries: 10);

  // If there"s a disjoint constraint on a package, then selecting other
  // versions of it is a waste of time: no possible versions can match. We need
  // to jump past it to the most recent package that affected the constraint.
  testResolve("backjump past failed package on disjoint constraint", {
    "myapp 0.0.0": {
      "a": "any",
      "foo": ">2.0.0"
    },
    "a 1.0.0": {
      "foo": "any" // ok
    },
    "a 2.0.0": {
      "foo": "<1.0.0" // disjoint with myapp"s constraint on foo
    },
    "foo 2.0.0": {},
    "foo 2.0.1": {},
    "foo 2.0.2": {},
    "foo 2.0.3": {},
    "foo 2.0.4": {}
  }, result: {
    "myapp from root": "0.0.0",
    "a": "1.0.0",
    "foo": "2.0.4"
  }, maxTries: 2);

  // This is a regression test for #18666. It was possible for the solver to
  // "forget" that a package had previously led to an error. In that case, it
  // would backtrack over the failed package instead of trying different
  // versions of it.
  testResolve("finds solution with less strict constraint", {
    "myapp 1.0.0": {
      "a": "any",
      "c": "any",
      "d": "any"
    },
    "a 2.0.0": {},
    "a 1.0.0": {},
    "b 1.0.0": {"a": "1.0.0"},
    "c 1.0.0": {"b": "any"},
    "d 2.0.0": {"myapp": "any"},
    "d 1.0.0": {"myapp": "<1.0.0"}
  }, result: {
    "myapp from root": "1.0.0",
    "a": "1.0.0",
    "b": "1.0.0",
    "c": "1.0.0",
    "d": "2.0.0"
  }, maxTries: 3);
}
*/
