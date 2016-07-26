package gps

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var fixtorun string

// TODO(sdboyer) regression test ensuring that locks with only revs for projects don't cause errors
func init() {
	flag.StringVar(&fixtorun, "gps.fix", "", "A single fixture to run in TestBasicSolves")
	overrideMkBridge()
}

// sets the mkBridge global func to one that allows virtualized RootDirs
func overrideMkBridge() {
	// For all tests, override the base bridge with the depspecBridge that skips
	// verifyRootDir calls
	mkBridge = func(s *solver, sm SourceManager) sourceBridge {
		return &depspecBridge{
			&bridge{
				sm:     sm,
				s:      s,
				vlists: make(map[ProjectRoot][]Version),
			},
		}
	}
}

var stderrlog = log.New(os.Stderr, "", 0)

func fixSolve(params SolveParameters, sm SourceManager) (Solution, error) {
	if testing.Verbose() {
		params.Trace = true
		params.TraceLogger = stderrlog
	}

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
			solveBasicsAndCheck(basicFixtures[n], t)
			if testing.Verbose() {
				// insert a line break between tests
				stderrlog.Println("")
			}
		}
	}
}

func solveBasicsAndCheck(fix basicFixture, t *testing.T) (res Solution, err error) {
	if testing.Verbose() {
		stderrlog.Printf("[[fixture %q]]", fix.n)
	}
	sm := newdepspecSM(fix.ds, nil)

	params := SolveParameters{
		RootDir:    string(fix.ds[0].n),
		ImportRoot: ProjectRoot(fix.ds[0].n),
		Manifest:   fix.rootmanifest(),
		Lock:       dummyLock{},
		Downgrade:  fix.downgrade,
		ChangeAll:  fix.changeall,
	}

	if fix.l != nil {
		params.Lock = fix.l
	}

	res, err = fixSolve(params, sm)

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
			solveBimodalAndCheck(bimodalFixtures[n], t)
			if testing.Verbose() {
				// insert a line break between tests
				stderrlog.Println("")
			}
		}
	}
}

func solveBimodalAndCheck(fix bimodalFixture, t *testing.T) (res Solution, err error) {
	if testing.Verbose() {
		stderrlog.Printf("[[fixture %q]]", fix.n)
	}
	sm := newbmSM(fix)

	params := SolveParameters{
		RootDir:    string(fix.ds[0].n),
		ImportRoot: ProjectRoot(fix.ds[0].n),
		Manifest:   fix.rootmanifest(),
		Lock:       dummyLock{},
		Downgrade:  fix.downgrade,
		ChangeAll:  fix.changeall,
	}

	if fix.l != nil {
		params.Lock = fix.l
	}

	res, err = fixSolve(params, sm)

	return fixtureSolveSimpleChecks(fix, res, err, t)
}

func fixtureSolveSimpleChecks(fix specfix, res Solution, err error, t *testing.T) (Solution, error) {
	if err != nil {
		errp := fix.expectErrs()
		if len(errp) == 0 {
			t.Errorf("(fixture: %q) Solver failed; error was type %T, text:\n%s", fix.name(), err, err)
			return res, err
		}

		switch fail := err.(type) {
		case *badOptsFailure:
			t.Errorf("(fixture: %q) Unexpected bad opts failure solve error: %s", fix.name(), err)
		case *noVersionError:
			if errp[0] != string(fail.pn.ProjectRoot) { // TODO(sdboyer) identifierify
				t.Errorf("(fixture: %q) Expected failure on project %s, but was on project %s", fix.name(), errp[0], fail.pn.ProjectRoot)
			}

			ep := make(map[string]struct{})
			for _, p := range errp[1:] {
				ep[p] = struct{}{}
			}

			found := make(map[string]struct{})
			for _, vf := range fail.fails {
				for _, f := range getFailureCausingProjects(vf.f) {
					found[f] = struct{}{}
				}
			}

			var missing []string
			var extra []string
			for p := range found {
				if _, has := ep[p]; !has {
					extra = append(extra, p)
				}
			}
			if len(extra) > 0 {
				t.Errorf("(fixture: %q) Expected solve failures due to projects %s, but solve failures also arose from %s", fix.name(), strings.Join(errp[1:], ", "), strings.Join(extra, ", "))
			}

			for p := range ep {
				if _, has := found[p]; !has {
					missing = append(missing, p)
				}
			}
			if len(missing) > 0 {
				t.Errorf("(fixture: %q) Expected solve failures due to projects %s, but %s had no failures", fix.name(), strings.Join(errp[1:], ", "), strings.Join(missing, ", "))
			}

		default:
			// TODO(sdboyer) round these out
			panic(fmt.Sprintf("unhandled solve failure type: %s", err))
		}
	} else if len(fix.expectErrs()) > 0 {
		t.Errorf("(fixture: %q) Solver succeeded, but expected failure", fix.name())
	} else {
		r := res.(solution)
		if fix.maxTries() > 0 && r.Attempts() > fix.maxTries() {
			t.Errorf("(fixture: %q) Solver completed in %v attempts, but expected %v or fewer", fix.name(), r.att, fix.maxTries())
		}

		// Dump result projects into a map for easier interrogation
		rp := make(map[string]Version)
		for _, p := range r.p {
			pa := p.toAtom()
			rp[string(pa.id.ProjectRoot)] = pa.v
		}

		fixlen, rlen := len(fix.solution()), len(rp)
		if fixlen != rlen {
			// Different length, so they definitely disagree
			t.Errorf("(fixture: %q) Solver reported %v package results, result expected %v", fix.name(), rlen, fixlen)
		}

		// Whether or not len is same, still have to verify that results agree
		// Walk through fixture/expected results first
		for p, v := range fix.solution() {
			if av, exists := rp[p]; !exists {
				t.Errorf("(fixture: %q) Project %q expected but missing from results", fix.name(), p)
			} else {
				// delete result from map so we skip it on the reverse pass
				delete(rp, p)
				if v != av {
					t.Errorf("(fixture: %q) Expected version %q of project %q, but actual version was %q", fix.name(), v, p, av)
				}
			}
		}

		// Now walk through remaining actual results
		for p, v := range rp {
			if fv, exists := fix.solution()[p]; !exists {
				t.Errorf("(fixture: %q) Unexpected project %q present in results", fix.name(), p)
			} else if v != fv {
				t.Errorf("(fixture: %q) Got version %q of project %q, but expected version was %q", fix.name(), v, p, fv)
			}
		}
	}

	return res, err
}

// This tests that, when a root lock is underspecified (has only a version) we
// don't allow a match on that version from a rev in the manifest. We may allow
// this in the future, but disallow it for now because going from an immutable
// requirement to a mutable lock automagically is a bad direction that could
// produce weird side effects.
func TestRootLockNoVersionPairMatching(t *testing.T) {
	fix := basicFixture{
		n: "does not pair bare revs in manifest with unpaired lock version",
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
			"bar 1.0.1",
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
		RootDir:    string(fix.ds[0].n),
		ImportRoot: ProjectRoot(fix.ds[0].n),
		Manifest:   fix.rootmanifest(),
		Lock:       l2,
	}

	res, err := fixSolve(params, sm)

	fixtureSolveSimpleChecks(fix, res, err, t)
}

func getFailureCausingProjects(err error) (projs []string) {
	switch e := err.(type) {
	case *noVersionError:
		projs = append(projs, string(e.pn.ProjectRoot)) // TODO(sdboyer) identifierify
	case *disjointConstraintFailure:
		for _, f := range e.failsib {
			projs = append(projs, string(f.depender.id.ProjectRoot))
		}
	case *versionNotAllowedFailure:
		for _, f := range e.failparent {
			projs = append(projs, string(f.depender.id.ProjectRoot))
		}
	case *constraintNotAllowedFailure:
		// No sane way of knowing why the currently selected version is
		// selected, so do nothing
	case *sourceMismatchFailure:
		projs = append(projs, string(e.prob.id.ProjectRoot))
		for _, c := range e.sel {
			projs = append(projs, string(c.depender.id.ProjectRoot))
		}
	case *checkeeHasProblemPackagesFailure:
		projs = append(projs, string(e.goal.id.ProjectRoot))
		for _, errdep := range e.failpkg {
			for _, atom := range errdep.deppers {
				projs = append(projs, string(atom.id.ProjectRoot))
			}
		}
	case *depHasProblemPackagesFailure:
		projs = append(projs, string(e.goal.depender.id.ProjectRoot), string(e.goal.dep.Ident.ProjectRoot))
	case *nonexistentRevisionFailure:
		projs = append(projs, string(e.goal.depender.id.ProjectRoot), string(e.goal.dep.Ident.ProjectRoot))
	default:
		panic(fmt.Sprintf("unknown failtype %T, msg: %s", err, err))
	}

	return
}

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

	params.ImportRoot = ProjectRoot(pn)
	params.Trace = true
	_, err = Prepare(params, sm)
	if err == nil {
		t.Errorf("Should have errored on trace with no logger")
	} else if !strings.Contains(err.Error(), "no logger provided") {
		t.Error("Prepare should have given error on missing trace logger, but gave:", err)
	}

	params.TraceLogger = log.New(ioutil.Discard, "", 0)
	_, err = Prepare(params, sm)
	if err != nil {
		t.Error("Basic conditions satisfied, prepare should have completed successfully, err as:", err)
	}

	// swap out the test mkBridge override temporarily, just to make sure we get
	// the right error
	mkBridge = func(s *solver, sm SourceManager) sourceBridge {
		return &bridge{
			sm:     sm,
			s:      s,
			vlists: make(map[ProjectRoot][]Version),
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
