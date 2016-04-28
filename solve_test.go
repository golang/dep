package vsolver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"
)

// TODO regression test ensuring that locks with only revs for projects don't cause errors

func TestBasicSolves(t *testing.T) {
	//solveAndBasicChecks(fixtures[8], t)
	for _, fix := range fixtures {
		solveAndBasicChecks(fix, t)
	}
}

func solveAndBasicChecks(fix fixture, t *testing.T) (res Result, err error) {
	sm := newdepspecSM(fix.ds)

	l := logrus.New()
	if testing.Verbose() {
		l.Level = logrus.DebugLevel
	} else {
		l.Level = logrus.WarnLevel
	}

	s := NewSolver(sm, l)

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

	res, err = s.Solve(o)
	if err != nil {
		if len(fix.errp) == 0 {
			t.Errorf("(fixture: %q) Solver failed; error was type %T, text: %q", fix.n, err, err)
		}

		switch fail := err.(type) {
		case *BadOptsFailure:
			t.Error("Unexpected bad opts failure solve error: %s", err)
		case *noVersionError:
			if fix.errp[0] != string(fail.pn) {
				t.Errorf("Expected failure on project %s, but was on project %s", fail.pn, fix.errp[0])
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
				t.Errorf("Expected solve failures due to projects %s, but solve failures also arose from %s", strings.Join(fix.errp[1:], ", "), strings.Join(extra, ", "))
			}

			for p, _ := range ep {
				if _, has := found[p]; !has {
					missing = append(missing, p)
				}
			}
			if len(missing) > 0 {
				t.Errorf("Expected solve failures due to projects %s, but %s had no failures", strings.Join(fix.errp[1:], ", "), strings.Join(missing, ", "))
			}

		default:
			// TODO round these out
			panic(fmt.Sprintf("unhandled solve failure type: %s", err))
		}
	} else if len(fix.errp) > 0 {
		t.Errorf("(fixture: %q) Solver succeeded, but expected failure")
	} else {
		r := res.(result)
		if fix.maxAttempts > 0 && r.att > fix.maxAttempts {
			t.Errorf("(fixture: %q) Solver completed in %v attempts, but expected %v or fewer", fix.n, r.att, fix.maxAttempts)
		}

		// Dump result projects into a map for easier interrogation
		rp := make(map[string]Version)
		for _, p := range r.p {
			pa := p.toAtom()
			rp[string(pa.Name)] = pa.Version
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

	return
}

func getFailureCausingProjects(err error) (projs []string) {
	switch e := err.(type) {
	case *noVersionError:
		projs = append(projs, string(e.pn))
	case *disjointConstraintFailure:
		for _, f := range e.failsib {
			projs = append(projs, string(f.Depender.Name))
		}
	case *versionNotAllowedFailure:
		for _, f := range e.failparent {
			projs = append(projs, string(f.Depender.Name))
		}
	case *constraintNotAllowedFailure:
		// No sane way of knowing why the currently selected version is
		// selected, so do nothing
	default:
		panic("unknown failtype")
	}

	return
}

func TestBadSolveOpts(t *testing.T) {
	sm := newdepspecSM(fixtures[0].ds)

	l := logrus.New()
	if testing.Verbose() {
		l.Level = logrus.DebugLevel
	}

	s := NewSolver(sm, l)

	o := SolveOpts{}
	_, err := s.Solve(o)
	if err == nil {
		t.Errorf("Should have errored on missing manifest")
	}

	p, _ := sm.GetProjectInfo(fixtures[0].ds[0].name)
	o.M = p.Manifest
	_, err = s.Solve(o)
	if err == nil {
		t.Errorf("Should have errored on empty root")
	}

	o.Root = "foo"
	_, err = s.Solve(o)
	if err == nil {
		t.Errorf("Should have errored on empty name")
	}

	o.N = "root"
	_, err = s.Solve(o)
	if err != nil {
		t.Errorf("Basic conditions satisfied, solve should have gone through")
	}
}
