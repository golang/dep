package vsolver

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver"
)

var regfrom = regexp.MustCompile(`^(\w*) from (\w*) ([0-9\.]*)`)

// nsvSplit splits an "info" string on " " into the pair of name and
// version/constraint, and returns each individually.
//
// This is for narrow use - panics if there are less than two resulting items in
// the slice.
func nsvSplit(info string) (id ProjectIdentifier, version string) {
	if strings.Contains(info, " from ") {
		parts := regfrom.FindStringSubmatch(info)
		info = parts[1] + " " + parts[3]
		id.NetworkName = parts[2]
	}

	s := strings.SplitN(info, " ", 2)
	if len(s) < 2 {
		panic(fmt.Sprintf("Malformed name/version info string '%s'", info))
	}

	id.LocalName, version = ProjectName(s[0]), s[1]
	if id.NetworkName == "" {
		id.NetworkName = string(id.LocalName)
	}
	return
}

// nsvrSplit splits an "info" string on " " into the triplet of name,
// version/constraint, and revision, and returns each individually.
//
// It will work fine if only name and version/constraint are provided.
//
// This is for narrow use - panics if there are less than two resulting items in
// the slice.
func nsvrSplit(info string) (id ProjectIdentifier, version string, revision Revision) {
	if strings.Contains(info, " from ") {
		parts := regfrom.FindStringSubmatch(info)
		info = parts[1] + " " + parts[3]
		id.NetworkName = parts[2]
	}

	s := strings.SplitN(info, " ", 3)
	if len(s) < 2 {
		panic(fmt.Sprintf("Malformed name/version info string '%s'", info))
	}

	id.LocalName, version = ProjectName(s[0]), s[1]
	if id.NetworkName == "" {
		id.NetworkName = string(id.LocalName)
	}

	if len(s) == 3 {
		revision = Revision(s[2])
	}
	return
}

// mksvpa - "make semver project atom"
//
// Splits the input string on a space, and uses the first two elements as the
// project name and constraint body, respectively.
func mksvpa(info string) ProjectAtom {
	id, ver, rev := nsvrSplit(info)

	_, err := semver.NewVersion(ver)
	if err != nil {
		// don't want to allow bad test data at this level, so just panic
		panic(fmt.Sprintf("Error when converting '%s' into semver: %s", ver, err))
	}

	var v Version
	v = NewVersion(ver)
	if rev != "" {
		v = v.(UnpairedVersion).Is(rev)
	}

	return ProjectAtom{
		Ident:   id,
		Version: v,
	}
}

// mkc - "make constraint"
func mkc(body string) Constraint {
	c, err := NewSemverConstraint(body)
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
	id, v := nsvSplit(info)

	return ProjectDep{
		Ident:      id,
		Constraint: mkc(v),
	}
}

type depspec struct {
	n       ProjectName
	v       Version
	deps    []ProjectDep
	devdeps []ProjectDep
	pkgs    []tpkg
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
	pa := mksvpa(pi)
	if string(pa.Ident.LocalName) != pa.Ident.NetworkName {
		panic("alternate source on self makes no sense")
	}

	ds := depspec{
		n: pa.Ident.LocalName,
		v: pa.Version,
	}

	for _, dep := range deps {
		var sl *[]ProjectDep
		if strings.HasPrefix(dep, "(dev) ") {
			dep = strings.TrimPrefix(dep, "(dev) ")
			sl = &ds.devdeps
		} else {
			sl = &ds.deps
		}

		*sl = append(*sl, mksvd(dep))
	}

	return ds
}

// mklock makes a fixLock, suitable to act as a lock file
func mklock(pairs ...string) fixLock {
	l := make(fixLock, 0)
	for _, s := range pairs {
		pa := mksvpa(s)
		l = append(l, NewLockedProject(pa.Ident.LocalName, pa.Version, pa.Ident.netName(), "", nil))
	}

	return l
}

// mkrevlock makes a fixLock, suitable to act as a lock file, with only a name
// and a rev
func mkrevlock(pairs ...string) fixLock {
	l := make(fixLock, 0)
	for _, s := range pairs {
		pa := mksvpa(s)
		l = append(l, NewLockedProject(pa.Ident.LocalName, pa.Version.(PairedVersion).Underlying(), pa.Ident.netName(), "", nil))
	}

	return l
}

// mkresults makes a result set
func mkresults(pairs ...string) map[string]Version {
	m := make(map[string]Version)
	for _, pair := range pairs {
		name, ver, rev := nsvrSplit(pair)

		var v Version
		v = NewVersion(ver)
		if rev != "" {
			v = v.(UnpairedVersion).Is(rev)
		}

		m[string(name.LocalName)] = v
	}

	return m
}

// computeReachMap takes a depspec and computes a reach map which is identical
// to the explicit depgraph.
func computeReachMap(ds []depspec) map[pident][]string {
	rm := make(map[pident][]string)

	for k, d := range ds {
		id := pident{
			n: d.n,
			v: d.v,
		}

		// Ensure we capture things even with no deps
		rm[id] = nil

		for _, dep := range d.deps {
			rm[id] = append(rm[id], string(dep.Ident.LocalName))
		}

		// first is root
		if k == 0 {
			for _, dep := range d.devdeps {
				rm[id] = append(rm[id], string(dep.Ident.LocalName))
			}
		}
	}

	return rm
}

type pident struct {
	n ProjectName
	v Version
}

type specfix interface {
	name() string
	specs() []depspec
	maxTries() int
	expectErrs() []string
	result() map[string]Version
}

type basicFixture struct {
	// name of this fixture datum
	n string
	// depspecs. always treat first as root
	ds []depspec
	// reachability map for each name
	rm map[pident][]string
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

func (f basicFixture) name() string {
	return f.n
}

func (f basicFixture) specs() []depspec {
	return f.ds
}

func (f basicFixture) maxTries() int {
	return f.maxAttempts
}

func (f basicFixture) expectErrs() []string {
	return f.errp
}

func (f basicFixture) result() map[string]Version {
	return f.r
}

var basicFixtures = []basicFixture{
	// basic fixtures
	{
		n: "no dependencies",
		ds: []depspec{
			dsv("root 0.0.0"),
		},
		r: mkresults(),
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
			"a 1.0.0",
			"b 1.0.0",
			"shared 3.6.9",
		),
	},
	{
		n: "downgrade on overlapping constraints",
		ds: []depspec{
			dsv("root 0.0.0", "a 1.0.0", "b 1.0.0"),
			dsv("a 1.0.0", "shared >=2.0.0, <=4.0.0"),
			dsv("b 1.0.0", "shared >=3.0.0, <5.0.0"),
			dsv("shared 2.0.0"),
			dsv("shared 3.0.0"),
			dsv("shared 3.6.9"),
			dsv("shared 4.0.0"),
			dsv("shared 5.0.0"),
		},
		r: mkresults(
			"a 1.0.0",
			"b 1.0.0",
			"shared 3.0.0",
		),
		downgrade: true,
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
			dsv("baz 1.0.0", "foo 2.0.0"),
		},
		r: mkresults(
			"foo 1.0.0",
			"bar 1.0.0",
		),
		maxAttempts: 2,
	},
	{
		n: "with mismatched net addrs",
		ds: []depspec{
			dsv("root 1.0.0", "foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.0", "bar from baz 1.0.0"),
			dsv("bar 1.0.0"),
		},
		// TODO ugh; do real error comparison instead of shitty abstraction
		errp: []string{"foo", "foo", "root"},
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
			"foo 1.0.1",
			"bar 1.0.1",
		),
	},
	{
		n: "upgrade through lock",
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
			"foo 1.0.2",
			"bar 1.0.2",
		),
		changeall: true,
	},
	{
		n: "downgrade through lock",
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
			"foo 1.0.0",
			"bar 1.0.0",
		),
		changeall: true,
		downgrade: true,
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
			dsv("baz 1.0.0 bazrev"),
		},
		l: mklock(
			"baz 1.0.0 bazrev",
		),
		r: mkresults(
			"foo 1.0.2",
			"bar 1.0.2",
		),
	},
	{
		n: "unlocks dependencies if necessary to ensure that a new dependency is satisfied",
		ds: []depspec{
			dsv("root 0.0.0", "foo *", "newdep *"),
			dsv("foo 1.0.0 foorev", "bar <2.0.0"),
			dsv("bar 1.0.0 barrev", "baz <2.0.0"),
			dsv("baz 1.0.0 bazrev", "qux <2.0.0"),
			dsv("qux 1.0.0 quxrev"),
			dsv("foo 2.0.0", "bar <3.0.0"),
			dsv("bar 2.0.0", "baz <3.0.0"),
			dsv("baz 2.0.0", "qux <3.0.0"),
			dsv("qux 2.0.0"),
			dsv("newdep 2.0.0", "baz >=1.5.0"),
		},
		l: mklock(
			"foo 1.0.0 foorev",
			"bar 1.0.0 barrev",
			"baz 1.0.0 bazrev",
			"qux 1.0.0 quxrev",
		),
		r: mkresults(
			"foo 2.0.0",
			"bar 2.0.0",
			"baz 2.0.0",
			"qux 1.0.0 quxrev",
			"newdep 2.0.0",
		),
		maxAttempts: 4,
	},
	{
		n: "locked atoms are matched on both local and net name",
		ds: []depspec{
			dsv("root 0.0.0", "foo *"),
			dsv("foo 1.0.0 foorev"),
			dsv("foo 2.0.0 foorev2"),
		},
		l: mklock(
			"foo from baz 1.0.0 foorev",
		),
		r: mkresults(
			"foo 2.0.0 foorev2",
		),
	},
	{
		n: "pairs bare revs in lock with versions",
		ds: []depspec{
			dsv("root 0.0.0", "foo ~1.0.1"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.1 foorev", "bar 1.0.1"),
			dsv("foo 1.0.2", "bar 1.0.2"),
			dsv("bar 1.0.0"),
			dsv("bar 1.0.1"),
			dsv("bar 1.0.2"),
		},
		l: mkrevlock(
			"foo 1.0.1 foorev", // mkrevlock drops the 1.0.1
		),
		r: mkresults(
			"foo 1.0.1 foorev",
			"bar 1.0.1",
		),
	},
	{
		n: "pairs bare revs in lock with all versions",
		ds: []depspec{
			dsv("root 0.0.0", "foo ~1.0.1"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.1 foorev", "bar 1.0.1"),
			dsv("foo 1.0.2 foorev", "bar 1.0.2"),
			dsv("bar 1.0.0"),
			dsv("bar 1.0.1"),
			dsv("bar 1.0.2"),
		},
		l: mkrevlock(
			"foo 1.0.1 foorev", // mkrevlock drops the 1.0.1
		),
		r: mkresults(
			"foo 1.0.2 foorev",
			"bar 1.0.1",
		),
	},
	{
		n: "does not pair bare revs in manifest with unpaired lock version",
		ds: []depspec{
			dsv("root 0.0.0", "foo ~1.0.1"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.1 foorev", "bar 1.0.1"),
			dsv("foo 1.0.2", "bar 1.0.2"),
			dsv("bar 1.0.0"),
			dsv("bar 1.0.1"),
			dsv("bar 1.0.2"),
		},
		l: mkrevlock(
			"foo 1.0.1 foorev", // mkrevlock drops the 1.0.1
		),
		r: mkresults(
			"foo 1.0.1 foorev",
			"bar 1.0.1",
		),
	},
	{
		n: "includes root package's dev dependencies",
		ds: []depspec{
			dsv("root 1.0.0", "(dev) foo 1.0.0", "(dev) bar 1.0.0"),
			dsv("foo 1.0.0"),
			dsv("bar 1.0.0"),
		},
		r: mkresults(
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
	{
		n: "no version that matches combined constraint",
		ds: []depspec{
			dsv("root 0.0.0", "foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.0", "shared >=2.0.0, <3.0.0"),
			dsv("bar 1.0.0", "shared >=2.9.0, <4.0.0"),
			dsv("shared 2.5.0"),
			dsv("shared 3.5.0"),
		},
		errp: []string{"shared", "foo", "bar"},
	},
	{
		n: "disjoint constraints",
		ds: []depspec{
			dsv("root 0.0.0", "foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.0", "shared <=2.0.0"),
			dsv("bar 1.0.0", "shared >3.0.0"),
			dsv("shared 2.0.0"),
			dsv("shared 4.0.0"),
		},
		//errp: []string{"shared", "foo", "bar"}, // dart's has this...
		errp: []string{"foo", "bar"},
	},
	{
		n: "no valid solution",
		ds: []depspec{
			dsv("root 0.0.0", "a *", "b *"),
			dsv("a 1.0.0", "b 1.0.0"),
			dsv("a 2.0.0", "b 2.0.0"),
			dsv("b 1.0.0", "a 2.0.0"),
			dsv("b 2.0.0", "a 1.0.0"),
		},
		errp:        []string{"b", "a"},
		maxAttempts: 2,
	},
	{
		n: "no version that matches while backtracking",
		ds: []depspec{
			dsv("root 0.0.0", "a *", "b >1.0.0"),
			dsv("a 1.0.0"),
			dsv("b 1.0.0"),
		},
		errp: []string{"b", "root"},
	},
	{
		// The latest versions of a and b disagree on c. An older version of either
		// will resolve the problem. This test validates that b, which is farther
		// in the dependency graph from myapp is downgraded first.
		n: "rolls back leaf versions first",
		ds: []depspec{
			dsv("root 0.0.0", "a *"),
			dsv("a 1.0.0", "b *"),
			dsv("a 2.0.0", "b *", "c 2.0.0"),
			dsv("b 1.0.0"),
			dsv("b 2.0.0", "c 1.0.0"),
			dsv("c 1.0.0"),
			dsv("c 2.0.0"),
		},
		r: mkresults(
			"a 2.0.0",
			"b 1.0.0",
			"c 2.0.0",
		),
		maxAttempts: 2,
	},
	{
		// Only one version of baz, so foo and bar will have to downgrade until they
		// reach it.
		n: "simple transitive",
		ds: []depspec{
			dsv("root 0.0.0", "foo *"),
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 2.0.0", "bar 2.0.0"),
			dsv("foo 3.0.0", "bar 3.0.0"),
			dsv("bar 1.0.0", "baz *"),
			dsv("bar 2.0.0", "baz 2.0.0"),
			dsv("bar 3.0.0", "baz 3.0.0"),
			dsv("baz 1.0.0"),
		},
		r: mkresults(
			"foo 1.0.0",
			"bar 1.0.0",
			"baz 1.0.0",
		),
		maxAttempts: 3,
	},
	{
		// Ensures the solver doesn"t exhaustively search all versions of b when
		// it's a-2.0.0 whose dependency on c-2.0.0-nonexistent led to the
		// problem. We make sure b has more versions than a so that the solver
		// tries a first since it sorts sibling dependencies by number of
		// versions.
		n: "simple transitive",
		ds: []depspec{
			dsv("root 0.0.0", "a *", "b *"),
			dsv("a 1.0.0", "c 1.0.0"),
			dsv("a 2.0.0", "c 2.0.0"),
			dsv("b 1.0.0"),
			dsv("b 2.0.0"),
			dsv("b 3.0.0"),
			dsv("c 1.0.0"),
		},
		r: mkresults(
			"a 1.0.0",
			"b 3.0.0",
			"c 1.0.0",
		),
		maxAttempts: 2,
	},
	{
		// Dependencies are ordered so that packages with fewer versions are
		// tried first. Here, there are two valid solutions (either a or b must
		// be downgraded once). The chosen one depends on which dep is traversed
		// first. Since b has fewer versions, it will be traversed first, which
		// means a will come later. Since later selections are revised first, a
		// gets downgraded.
		n: "traverse into package with fewer versions first",
		ds: []depspec{
			dsv("root 0.0.0", "a *", "b *"),
			dsv("a 1.0.0", "c *"),
			dsv("a 2.0.0", "c *"),
			dsv("a 3.0.0", "c *"),
			dsv("a 4.0.0", "c *"),
			dsv("a 5.0.0", "c 1.0.0"),
			dsv("b 1.0.0", "c *"),
			dsv("b 2.0.0", "c *"),
			dsv("b 3.0.0", "c *"),
			dsv("b 4.0.0", "c 2.0.0"),
			dsv("c 1.0.0"),
			dsv("c 2.0.0"),
		},
		r: mkresults(
			"a 4.0.0",
			"b 4.0.0",
			"c 2.0.0",
		),
		maxAttempts: 2,
	},
	{
		// This is similar to the preceding fixture. When getting the number of
		// versions of a package to determine which to traverse first, versions
		// that are disallowed by the root package"s constraints should not be
		// considered. Here, foo has more versions of bar in total (4), but
		// fewer that meet myapp"s constraints (only 2). There is no solution,
		// but we will do less backtracking if foo is tested first.
		n: "traverse into package with fewer versions first",
		ds: []depspec{
			dsv("root 0.0.0", "foo *", "bar *"),
			dsv("foo 1.0.0", "none 2.0.0"),
			dsv("foo 2.0.0", "none 2.0.0"),
			dsv("foo 3.0.0", "none 2.0.0"),
			dsv("foo 4.0.0", "none 2.0.0"),
			dsv("bar 1.0.0"),
			dsv("bar 2.0.0"),
			dsv("bar 3.0.0"),
			dsv("none 1.0.0"),
		},
		errp:        []string{"none", "foo"},
		maxAttempts: 2,
	},
	{
		// If there"s a disjoint constraint on a package, then selecting other
		// versions of it is a waste of time: no possible versions can match. We
		// need to jump past it to the most recent package that affected the
		// constraint.
		n: "backjump past failed package on disjoint constraint",
		ds: []depspec{
			dsv("root 0.0.0", "a *", "foo *"),
			dsv("a 1.0.0", "foo *"),
			dsv("a 2.0.0", "foo <1.0.0"),
			dsv("foo 2.0.0"),
			dsv("foo 2.0.1"),
			dsv("foo 2.0.2"),
			dsv("foo 2.0.3"),
			dsv("foo 2.0.4"),
			dsv("none 1.0.0"),
		},
		r: mkresults(
			"a 1.0.0",
			"foo 2.0.4",
		),
		maxAttempts: 2,
	},
	// TODO add fixture that tests proper handling of loops via aliases (where
	// a project that wouldn't be a loop is aliased to a project that is a loop)
}

func init() {
	// This sets up a hundred versions of foo and bar, 0.0.0 through 9.9.0. Each
	// version of foo depends on a baz with the same major version. Each version
	// of bar depends on a baz with the same minor version. There is only one
	// version of baz, 0.0.0, so only older versions of foo and bar will
	// satisfy it.
	fix := basicFixture{
		n: "complex backtrack",
		ds: []depspec{
			dsv("root 0.0.0", "foo *", "bar *"),
			dsv("baz 0.0.0"),
		},
		r: mkresults(
			"foo 0.9.0",
			"bar 9.0.0",
			"baz 0.0.0",
		),
		maxAttempts: 10,
	}

	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			fix.ds = append(fix.ds, dsv(fmt.Sprintf("foo %v.%v.0", i, j), fmt.Sprintf("baz %v.0.0", i)))
			fix.ds = append(fix.ds, dsv(fmt.Sprintf("bar %v.%v.0", i, j), fmt.Sprintf("baz 0.%v.0", j)))
		}
	}

	basicFixtures = append(basicFixtures, fix)

	for k, f := range basicFixtures {
		f.rm = computeReachMap(f.ds)
		basicFixtures[k] = f
	}
}

type depspecSourceManager struct {
	specs  []depspec
	rm     map[pident][]string
	sortup bool
}

type fixSM interface {
	SourceManager
	rootSpec() depspec
	allSpecs() []depspec
}

var _ fixSM = &depspecSourceManager{}

func newdepspecSM(ds []depspec, rm map[pident][]string) *depspecSourceManager {
	return &depspecSourceManager{
		specs: ds,
		rm:    rm,
	}
}

func (sm *depspecSourceManager) GetProjectInfo(n ProjectName, v Version) (ProjectInfo, error) {
	for _, ds := range sm.specs {
		if n == ds.n && v.Matches(ds.v) {
			return ProjectInfo{
				N:        ds.n,
				V:        ds.v,
				Manifest: ds,
				Lock:     dummyLock{},
			}, nil
		}
	}

	// TODO proper solver-type errors
	return ProjectInfo{}, fmt.Errorf("Project '%s' at version '%s' could not be found", n, v)
}

func (sm *depspecSourceManager) ExternalReach(n ProjectName, v Version) (map[string][]string, error) {
	id := pident{n: n, v: v}
	if r, exists := sm.rm[id]; exists {
		m := make(map[string][]string)
		m[string(n)] = r

		return m, nil
	}
	return nil, fmt.Errorf("No reach data for %s at version %s", n, v)
}

func (sm *depspecSourceManager) ListExternal(n ProjectName, v Version) ([]string, error) {
	// This should only be called for the root
	id := pident{n: n, v: v}
	if r, exists := sm.rm[id]; exists {
		return r, nil
	}
	return nil, fmt.Errorf("No reach data for %s at version %s", n, v)
}

func (sm *depspecSourceManager) ListPackages(n ProjectName, v Version) (map[string]string, error) {
	m := make(map[string]string)
	m[string(n)] = string(n)

	return m, nil
}

func (sm *depspecSourceManager) ListVersions(name ProjectName) (pi []Version, err error) {
	for _, ds := range sm.specs {
		if name == ds.n {
			pi = append(pi, ds.v)
		}
	}

	if len(pi) == 0 {
		err = fmt.Errorf("Project '%s' could not be found", name)
	}

	return
}

func (sm *depspecSourceManager) RepoExists(name ProjectName) (bool, error) {
	for _, ds := range sm.specs {
		if name == ds.n {
			return true, nil
		}
	}

	return false, nil
}

func (sm *depspecSourceManager) VendorCodeExists(name ProjectName) (bool, error) {
	return false, nil
}

func (sm *depspecSourceManager) Release() {}

func (sm *depspecSourceManager) ExportProject(n ProjectName, v Version, to string) error {
	return fmt.Errorf("dummy sm doesn't support exporting")
}

func (sm *depspecSourceManager) rootSpec() depspec {
	return sm.specs[0]
}

func (sm *depspecSourceManager) allSpecs() []depspec {
	return sm.specs
}

type depspecBridge struct {
	*bridge
}

// override computeRootReach() on bridge to read directly out of the depspecs
func (b *depspecBridge) computeRootReach(path string) ([]string, error) {
	// This only gets called for the root project, so grab that one off the test
	// source manager
	dsm := b.sm.(fixSM)
	root := dsm.rootSpec()
	if string(root.n) != path {
		return nil, fmt.Errorf("Expected only root project %q to computeRootReach(), got %q", root.n, path)
	}

	return dsm.ListExternal(root.n, root.v)
}

// override verifyRoot() on bridge to prevent any filesystem interaction
func (b *depspecBridge) verifyRoot(path string) error {
	root := b.sm.(fixSM).rootSpec()
	if string(root.n) != path {
		return fmt.Errorf("Expected only root project %q to computeRootReach(), got %q", root.n, path)
	}

	return nil
}

func (b *depspecBridge) externalReach(id ProjectIdentifier, v Version) (map[string][]string, error) {
	return b.sm.ExternalReach(b.key(id), v)
}
func (b *depspecBridge) listPackages(id ProjectIdentifier, v Version) (map[string]string, error) {
	return b.sm.ListPackages(b.key(id), v)
}

// override deduceRemoteRepo on bridge to make all our pkg/project mappings work
// as expected
func (b *depspecBridge) deduceRemoteRepo(path string) (*remoteRepo, error) {
	for _, ds := range b.sm.(fixSM).allSpecs() {
		n := string(ds.n)
		if strings.HasPrefix(path, n) {
			return &remoteRepo{
				Base:   n,
				RelPkg: strings.TrimPrefix(path, n+"/"),
			}, nil
		}
	}
	return nil, fmt.Errorf("Could not find %s, or any parent, in list of known fixtures", path)
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
	return ds.n
}

type fixLock []LockedProject

func (fixLock) SolverVersion() string {
	return "-1"
}

// impl Lock interface
func (fixLock) InputHash() []byte {
	return []byte("fooooorooooofooorooofoo")
}

// impl Lock interface
func (l fixLock) Projects() []LockedProject {
	return l
}

type dummyLock struct{}

// impl Lock interface
func (_ dummyLock) SolverVersion() string {
	return "-1"
}

// impl Lock interface
func (_ dummyLock) InputHash() []byte {
	return []byte("fooooorooooofooorooofoo")
}

// impl Lock interface
func (_ dummyLock) Projects() []LockedProject {
	return nil
}

// We've borrowed this bestiary from pub's tests:
// https://github.com/dart-lang/pub/blob/master/test/version_solver_test.dart

// TODO finish converting all of these

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
}
*/
