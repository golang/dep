package gps

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/sdboyer/gps/internal"
	"github.com/sdboyer/gps/pkgtree"
)

var fixtorun string

// TODO(sdboyer) regression test ensuring that locks with only revs for projects don't cause errors
func init() {
	flag.StringVar(&fixtorun, "gps.fix", "", "A single fixture to run in TestBasicSolves or TestBimodalSolves")
	mkBridge(nil, nil, false)
	overrideMkBridge()
	overrideIsStdLib()
}

// sets the mkBridge global func to one that allows virtualized RootDirs
func overrideMkBridge() {
	// For all tests, override the base bridge with the depspecBridge that skips
	// verifyRootDir calls
	mkBridge = func(s *solver, sm SourceManager, down bool) sourceBridge {
		return &depspecBridge{
			&bridge{
				sm:     sm,
				s:      s,
				down:   down,
				vlists: make(map[ProjectIdentifier][]Version),
			},
		}
	}
}

// sets the isStdLib func to always return false, otherwise it would identify
// pretty much all of our fixtures as being stdlib and skip everything
func overrideIsStdLib() {
	internal.IsStdLib = func(path string) bool {
		return false
	}
}

type testlogger struct {
	*testing.T
}

func (t testlogger) Write(b []byte) (n int, err error) {
	str := string(b)
	if len(str) == 0 {
		return 0, nil
	}

	for _, part := range strings.Split(str, "\n") {
		str := strings.TrimRightFunc(part, unicode.IsSpace)
		if len(str) != 0 {
			t.T.Log(str)
		}
	}
	return len(b), err
}

func fixSolve(params SolveParameters, sm SourceManager, t *testing.T) (Solution, error) {
	// Trace unconditionally; by passing the trace through t.Log(), the testing
	// system will decide whether or not to actually show the output (based on
	// -v, or selectively on test failure).
	params.Trace = true
	params.TraceLogger = log.New(testlogger{T: t}, "", 0)

	s, err := Prepare(params, sm)
	if err != nil {
		return nil, err
	}

	return s.Solve()
}

// Test all the basic table fixtures.
//
// Or, just the one named in the fix arg.
func TestBasicSolves(t *testing.T) {
	if fixtorun != "" {
		if fix, exists := basicFixtures[fixtorun]; exists {
			solveBasicsAndCheck(fix, t)
		}
	} else {
		// sort them by their keys so we get stable output
		var names []string
		for n := range basicFixtures {
			names = append(names, n)
		}

		sort.Strings(names)
		for _, n := range names {
			t.Run(n, func(t *testing.T) {
				//t.Parallel() // until trace output is fixed in parallel
				solveBasicsAndCheck(basicFixtures[n], t)
			})
		}
	}
}

func solveBasicsAndCheck(fix basicFixture, t *testing.T) (res Solution, err error) {
	sm := newdepspecSM(fix.ds, nil)

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        fix.rootmanifest(),
		Lock:            dummyLock{},
		Downgrade:       fix.downgrade,
		ChangeAll:       fix.changeall,
		ToChange:        fix.changelist,
		ProjectAnalyzer: naiveAnalyzer{},
	}

	if fix.l != nil {
		params.Lock = fix.l
	}

	res, err = fixSolve(params, sm, t)

	return fixtureSolveSimpleChecks(fix, res, err, t)
}

// Test all the bimodal table fixtures.
//
// Or, just the one named in the fix arg.
func TestBimodalSolves(t *testing.T) {
	if fixtorun != "" {
		if fix, exists := bimodalFixtures[fixtorun]; exists {
			solveBimodalAndCheck(fix, t)
		}
	} else {
		// sort them by their keys so we get stable output
		var names []string
		for n := range bimodalFixtures {
			names = append(names, n)
		}

		sort.Strings(names)
		for _, n := range names {
			t.Run(n, func(t *testing.T) {
				//t.Parallel() // until trace output is fixed in parallel
				solveBimodalAndCheck(bimodalFixtures[n], t)
			})
		}
	}
}

func solveBimodalAndCheck(fix bimodalFixture, t *testing.T) (res Solution, err error) {
	sm := newbmSM(fix)

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        fix.rootmanifest(),
		Lock:            dummyLock{},
		Downgrade:       fix.downgrade,
		ChangeAll:       fix.changeall,
		ProjectAnalyzer: naiveAnalyzer{},
	}

	if fix.l != nil {
		params.Lock = fix.l
	}

	res, err = fixSolve(params, sm, t)

	return fixtureSolveSimpleChecks(fix, res, err, t)
}

func fixtureSolveSimpleChecks(fix specfix, soln Solution, err error, t *testing.T) (Solution, error) {
	ppi := func(id ProjectIdentifier) string {
		// need this so we can clearly tell if there's a Source or not
		if id.Source == "" {
			return string(id.ProjectRoot)
		}
		return fmt.Sprintf("%s (from %s)", id.ProjectRoot, id.Source)
	}

	pv := func(v Version) string {
		if pv, ok := v.(PairedVersion); ok {
			return fmt.Sprintf("%s (%s)", pv.Unpair(), pv.Underlying())
		}
		return v.String()
	}

	fixfail := fix.failure()
	if err != nil {
		if fixfail == nil {
			t.Errorf("Solve failed unexpectedly:\n%s", err)
		} else if !reflect.DeepEqual(fixfail, err) {
			// TODO(sdboyer) reflect.DeepEqual works for now, but once we start
			// modeling more complex cases, this should probably become more robust
			t.Errorf("Failure mismatch:\n\t(GOT): %s\n\t(WNT): %s", err, fixfail)
		}
	} else if fixfail != nil {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Solver succeeded, but expecting failure:\n%s\nProjects in solution:", fixfail)
		for _, p := range soln.Projects() {
			fmt.Fprintf(&buf, "\n\t- %s at %s", ppi(p.Ident()), p.Version())
		}
		t.Error(buf.String())
	} else {
		r := soln.(solution)
		if fix.maxTries() > 0 && r.Attempts() > fix.maxTries() {
			t.Errorf("Solver completed in %v attempts, but expected %v or fewer", r.att, fix.maxTries())
		}

		// Dump result projects into a map for easier interrogation
		rp := make(map[ProjectIdentifier]LockedProject)
		for _, lp := range r.p {
			rp[lp.pi] = lp
		}

		fixlen, rlen := len(fix.solution()), len(rp)
		if fixlen != rlen {
			// Different length, so they definitely disagree
			t.Errorf("Solver reported %v package results, result expected %v", rlen, fixlen)
		}

		// Whether or not len is same, still have to verify that results agree
		// Walk through fixture/expected results first
		for id, flp := range fix.solution() {
			if lp, exists := rp[id]; !exists {
				t.Errorf("Project %q expected but missing from results", ppi(id))
			} else {
				// delete result from map so we skip it on the reverse pass
				delete(rp, id)
				if flp.Version() != lp.Version() {
					t.Errorf("Expected version %q of project %q, but actual version was %q", pv(flp.Version()), ppi(id), pv(lp.Version()))
				}

				if !reflect.DeepEqual(lp.pkgs, flp.pkgs) {
					t.Errorf("Package list was not not as expected for project %s@%s:\n\t(GOT) %s\n\t(WNT) %s", ppi(id), pv(lp.Version()), lp.pkgs, flp.pkgs)
				}
			}
		}

		// Now walk through remaining actual results
		for id, lp := range rp {
			if _, exists := fix.solution()[id]; !exists {
				t.Errorf("Unexpected project %s@%s present in results, with pkgs:\n\t%s", ppi(id), pv(lp.Version()), lp.pkgs)
			}
		}
	}

	return soln, err
}

// This tests that, when a root lock is underspecified (has only a version) we
// don't allow a match on that version from a rev in the manifest. We may allow
// this in the future, but disallow it for now because going from an immutable
// requirement to a mutable lock automagically is a bad direction that could
// produce weird side effects.
func TestRootLockNoVersionPairMatching(t *testing.T) {
	fix := basicFixture{
		n: "does not match unpaired lock versions with paired real versions",
		ds: []depspec{
			mkDepspec("root 0.0.0", "foo *"), // foo's constraint rewritten below to foorev
			mkDepspec("foo 1.0.0", "bar 1.0.0"),
			mkDepspec("foo 1.0.1 foorev", "bar 1.0.1"),
			mkDepspec("foo 1.0.2 foorev", "bar 1.0.2"),
			mkDepspec("bar 1.0.0"),
			mkDepspec("bar 1.0.1"),
			mkDepspec("bar 1.0.2"),
		},
		l: mklock(
			"foo 1.0.1",
		),
		r: mksolution(
			"foo 1.0.2 foorev",
			"bar 1.0.2",
		),
	}

	pd := fix.ds[0].deps[0]
	pd.Constraint = Revision("foorev")
	fix.ds[0].deps[0] = pd

	sm := newdepspecSM(fix.ds, nil)

	l2 := make(fixLock, 1)
	copy(l2, fix.l)
	l2[0].v = nil

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        fix.rootmanifest(),
		Lock:            l2,
		ProjectAnalyzer: naiveAnalyzer{},
	}

	res, err := fixSolve(params, sm, t)

	fixtureSolveSimpleChecks(fix, res, err, t)
}

// TestBadSolveOpts exercises the different possible inputs to a solver that can
// be determined as invalid in Prepare(), without any further work
func TestBadSolveOpts(t *testing.T) {
	pn := strconv.FormatInt(rand.Int63(), 36)
	fix := basicFixtures["no dependencies"]
	fix.ds[0].n = ProjectRoot(pn)

	sm := newdepspecSM(fix.ds, nil)
	params := SolveParameters{}

	_, err := Prepare(params, nil)
	if err == nil {
		t.Errorf("Prepare should have errored on nil SourceManager")
	} else if !strings.Contains(err.Error(), "non-nil SourceManager") {
		t.Error("Prepare should have given error on nil SourceManager, but gave:", err)
	}

	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Prepare should have errored without ProjectAnalyzer")
	} else if !strings.Contains(err.Error(), "must provide a ProjectAnalyzer") {
		t.Error("Prepare should have given error without ProjectAnalyzer, but gave:", err)
	}

	params.ProjectAnalyzer = naiveAnalyzer{}
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Prepare should have errored on empty root")
	} else if !strings.Contains(err.Error(), "non-empty root directory") {
		t.Error("Prepare should have given error on empty root, but gave:", err)
	}

	params.RootDir = pn
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Prepare should have errored on empty name")
	} else if !strings.Contains(err.Error(), "non-empty import root") {
		t.Error("Prepare should have given error on empty import root, but gave:", err)
	}

	params.RootPackageTree = pkgtree.PackageTree{
		ImportRoot: pn,
	}
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Prepare should have errored on empty name")
	} else if !strings.Contains(err.Error(), "at least one package") {
		t.Error("Prepare should have given error on empty import root, but gave:", err)
	}

	params.RootPackageTree = pkgtree.PackageTree{
		ImportRoot: pn,
		Packages: map[string]pkgtree.PackageOrErr{
			pn: {
				P: pkgtree.Package{
					ImportPath: pn,
					Name:       pn,
				},
			},
		},
	}
	params.Trace = true
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on trace with no logger")
	} else if !strings.Contains(err.Error(), "no logger provided") {
		t.Error("Prepare should have given error on missing trace logger, but gave:", err)
	}
	params.TraceLogger = log.New(ioutil.Discard, "", 0)

	params.Manifest = simpleRootManifest{
		ovr: ProjectConstraints{
			ProjectRoot("foo"): ProjectProperties{},
		},
	}
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on override with empty ProjectProperties")
	} else if !strings.Contains(err.Error(), "foo, but without any non-zero properties") {
		t.Error("Prepare should have given error override with empty ProjectProperties, but gave:", err)
	}

	params.Manifest = simpleRootManifest{
		ig:  map[string]bool{"foo": true},
		req: map[string]bool{"foo": true},
	}
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on pkg both ignored and required")
	} else if !strings.Contains(err.Error(), "was given as both a required and ignored package") {
		t.Error("Prepare should have given error with single ignore/require conflict error, but gave:", err)
	}

	params.Manifest = simpleRootManifest{
		ig:  map[string]bool{"foo": true, "bar": true},
		req: map[string]bool{"foo": true, "bar": true},
	}
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on pkg both ignored and required")
	} else if !strings.Contains(err.Error(), "multiple packages given as both required and ignored:") {
		t.Error("Prepare should have given error with multiple ignore/require conflict error, but gave:", err)
	}
	params.Manifest = nil

	params.ToChange = []ProjectRoot{"foo"}
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on non-empty ToChange without a lock provided")
	} else if !strings.Contains(err.Error(), "update specifically requested for") {
		t.Error("Prepare should have given error on ToChange without Lock, but gave:", err)
	}

	params.Lock = safeLock{
		p: []LockedProject{
			NewLockedProject(mkPI("bar"), Revision("makebelieve"), nil),
		},
	}
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on ToChange containing project not in lock")
	} else if !strings.Contains(err.Error(), "cannot update foo as it is not in the lock") {
		t.Error("Prepare should have given error on ToChange with item not present in Lock, but gave:", err)
	}

	params.Lock, params.ToChange = nil, nil
	_, err = Prepare(params, sm)
	if err != nil {
		t.Error("Basic conditions satisfied, prepare should have completed successfully, err as:", err)
	}

	// swap out the test mkBridge override temporarily, just to make sure we get
	// the right error
	mkBridge = func(s *solver, sm SourceManager, down bool) sourceBridge {
		return &bridge{
			sm:     sm,
			s:      s,
			down:   down,
			vlists: make(map[ProjectIdentifier][]Version),
		}
	}

	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on nonexistent root")
	} else if !strings.Contains(err.Error(), "could not read project root") {
		t.Error("Prepare should have given error nonexistent project root dir, but gave:", err)
	}

	// Pointing it at a file should also be an err
	params.RootDir = "solve_test.go"
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on file for RootDir")
	} else if !strings.Contains(err.Error(), "is a file, not a directory") {
		t.Error("Prepare should have given error on file as RootDir, but gave:", err)
	}

	// swap them back...not sure if this matters, but just in case
	overrideMkBridge()
}
