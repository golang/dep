package vsolver

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
)

var fixtorun string

// TODO regression test ensuring that locks with only revs for projects don't cause errors
func init() {
	flag.StringVar(&fixtorun, "vsolver.fix", "", "A single fixture to run in TestBasicSolves")
}

var stderrlog = log.New(os.Stderr, "", 0)

func fixSolve(o SolveOpts, sm SourceManager) (Result, error) {
	if testing.Verbose() {
		o.Trace = true
		o.TraceLogger = stderrlog
	}

	s, err := prepareSolver(o, sm)
	if err != nil {
		return nil, err
	}

	fixb := &depspecBridge{
		s.b.(*bridge),
	}
	s.b = fixb

	return s.run()
}

func TestBasicSolves(t *testing.T) {
	for _, fix := range fixtures {
		if fixtorun == "" || fixtorun == fix.n {
			solveAndBasicChecks(fix, t)
			if testing.Verbose() {
				// insert a line break between tests
				stderrlog.Println("")
			}
		}
	}
}

func solveAndBasicChecks(fix fixture, t *testing.T) (res Result, err error) {
	sm := newdepspecSM(fix.ds, fix.rm)

	o := SolveOpts{
		Root:      string(fix.ds[0].Name()),
		N:         ProjectName(fix.ds[0].Name()),
		M:         fix.ds[0],
		L:         dummyLock{},
		Downgrade: fix.downgrade,
		ChangeAll: fix.changeall,
	}

	if fix.l != nil {
		o.L = fix.l
	}

	res, err = fixSolve(o, sm)

	return fixtureSolveBasicChecks(fix, res, err, t)
}

func fixtureSolveBasicChecks(fix fixture, res Result, err error, t *testing.T) (Result, error) {
	if err != nil {
		if len(fix.errp) == 0 {
			t.Errorf("(fixture: %q) Solver failed; error was type %T, text: %q", fix.n, err, err)
			return res, err
		}

		switch fail := err.(type) {
		case *BadOptsFailure:
			t.Errorf("(fixture: %q) Unexpected bad opts failure solve error: %s", fix.n, err)
		case *noVersionError:
			if fix.errp[0] != string(fail.pn.LocalName) { // TODO identifierify
				t.Errorf("(fixture: %q) Expected failure on project %s, but was on project %s", fix.n, fail.pn.LocalName, fix.errp[0])
			}

			ep := make(map[string]struct{})
			for _, p := range fix.errp[1:] {
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
			for p, _ := range found {
				if _, has := ep[p]; !has {
					extra = append(extra, p)
				}
			}
			if len(extra) > 0 {
				t.Errorf("(fixture: %q) Expected solve failures due to projects %s, but solve failures also arose from %s", fix.n, strings.Join(fix.errp[1:], ", "), strings.Join(extra, ", "))
			}

			for p, _ := range ep {
				if _, has := found[p]; !has {
					missing = append(missing, p)
				}
			}
			if len(missing) > 0 {
				t.Errorf("(fixture: %q) Expected solve failures due to projects %s, but %s had no failures", fix.n, strings.Join(fix.errp[1:], ", "), strings.Join(missing, ", "))
			}

		default:
			// TODO round these out
			panic(fmt.Sprintf("unhandled solve failure type: %s", err))
		}
	} else if len(fix.errp) > 0 {
		t.Errorf("(fixture: %q) Solver succeeded, but expected failure", fix.n)
	} else {
		r := res.(result)
		if fix.maxAttempts > 0 && r.att > fix.maxAttempts {
			t.Errorf("(fixture: %q) Solver completed in %v attempts, but expected %v or fewer", fix.n, r.att, fix.maxAttempts)
		}

		// Dump result projects into a map for easier interrogation
		rp := make(map[string]Version)
		for _, p := range r.p {
			pa := p.toAtom()
			rp[string(pa.Ident.LocalName)] = pa.Version
		}

		fixlen, rlen := len(fix.r), len(rp)
		if fixlen != rlen {
			// Different length, so they definitely disagree
			t.Errorf("(fixture: %q) Solver reported %v package results, result expected %v", fix.n, rlen, fixlen)
		}

		// Whether or not len is same, still have to verify that results agree
		// Walk through fixture/expected results first
		for p, v := range fix.r {
			if av, exists := rp[p]; !exists {
				t.Errorf("(fixture: %q) Project %q expected but missing from results", fix.n, p)
			} else {
				// delete result from map so we skip it on the reverse pass
				delete(rp, p)
				if v != av {
					t.Errorf("(fixture: %q) Expected version %q of project %q, but actual version was %q", fix.n, v, p, av)
				}
			}
		}

		// Now walk through remaining actual results
		for p, v := range rp {
			if fv, exists := fix.r[p]; !exists {
				t.Errorf("(fixture: %q) Unexpected project %q present in results", fix.n, p)
			} else if v != fv {
				t.Errorf("(fixture: %q) Got version %q of project %q, but expected version was %q", fix.n, v, p, fv)
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
	fix := fixture{
		n: "does not pair bare revs in manifest with unpaired lock version",
		ds: []depspec{
			dsv("root 0.0.0", "foo *"), // foo's constraint rewritten below to foorev
			dsv("foo 1.0.0", "bar 1.0.0"),
			dsv("foo 1.0.1 foorev", "bar 1.0.1"),
			dsv("foo 1.0.2 foorev", "bar 1.0.2"),
			dsv("bar 1.0.0"),
			dsv("bar 1.0.1"),
			dsv("bar 1.0.2"),
		},
		l: mklock(
			"foo 1.0.1",
		),
		r: mkresults(
			"foo 1.0.2 foorev",
			"bar 1.0.1",
		),
	}

	pd := fix.ds[0].deps[0]
	pd.Constraint = Revision("foorev")
	fix.ds[0].deps[0] = pd
	fix.rm = computeReachMap(fix.ds)

	sm := newdepspecSM(fix.ds, fix.rm)

	l2 := make(fixLock, 1)
	copy(l2, fix.l)
	l2[0].v = nil

	o := SolveOpts{
		Root: string(fix.ds[0].Name()),
		N:    ProjectName(fix.ds[0].Name()),
		M:    fix.ds[0],
		L:    l2,
	}

	res, err := fixSolve(o, sm)

	fixtureSolveBasicChecks(fix, res, err, t)
}

func getFailureCausingProjects(err error) (projs []string) {
	switch e := err.(type) {
	case *noVersionError:
		projs = append(projs, string(e.pn.LocalName)) // TODO identifierify
	case *disjointConstraintFailure:
		for _, f := range e.failsib {
			projs = append(projs, string(f.Depender.Ident.LocalName))
		}
	case *versionNotAllowedFailure:
		for _, f := range e.failparent {
			projs = append(projs, string(f.Depender.Ident.LocalName))
		}
	case *constraintNotAllowedFailure:
		// No sane way of knowing why the currently selected version is
		// selected, so do nothing
	case *sourceMismatchFailure:
		projs = append(projs, string(e.prob.Ident.LocalName))
		for _, c := range e.sel {
			projs = append(projs, string(c.Depender.Ident.LocalName))
		}
	default:
		panic("unknown failtype")
	}

	return
}

func TestBadSolveOpts(t *testing.T) {
	sm := newdepspecSM(fixtures[0].ds, fixtures[0].rm)

	o := SolveOpts{}
	_, err := fixSolve(o, sm)
	if err == nil {
		t.Errorf("Should have errored on missing manifest")
	}

	p, _ := sm.GetProjectInfo(fixtures[0].ds[0].n, fixtures[0].ds[0].v)
	o.M = p.Manifest
	_, err = fixSolve(o, sm)
	if err == nil {
		t.Errorf("Should have errored on empty root")
	}

	o.Root = "root"
	_, err = fixSolve(o, sm)
	if err == nil {
		t.Errorf("Should have errored on empty name")
	}

	o.N = "root"
	_, err = fixSolve(o, sm)
	if err != nil {
		t.Errorf("Basic conditions satisfied, solve should have gone through, err was %s", err)
	}

	o.Trace = true
	_, err = fixSolve(o, sm)
	if err == nil {
		t.Errorf("Should have errored on trace with no logger")
	}

	o.TraceLogger = log.New(ioutil.Discard, "", 0)
	_, err = fixSolve(o, sm)
	if err != nil {
		t.Errorf("Basic conditions re-satisfied, solve should have gone through, err was %s", err)
	}

}
