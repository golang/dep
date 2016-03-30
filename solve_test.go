package vsolver

import "testing"

func TestBasicSolves(t *testing.T) {
	solveAndBasicChecks(0, t)
	solveAndBasicChecks(1, t)
	solveAndBasicChecks(2, t)
}

func solveAndBasicChecks(fixnum int, t *testing.T) Result {
	fix := fixtures[fixnum]
	sm := &depspecSourceManager{specs: fix.ds}
	s := NewSolver(sm)

	p, err := sm.GetProjectInfo(fix.ds[0].name)
	if err != nil {
		t.Error("wtf, couldn't find root project")
		t.FailNow()
	}
	result := s.Solve(p, nil)

	if result.SolveFailure != nil {
		t.Errorf("(fixture: %q) - Solver failed; error was type %T, text: %q", fix.n, result.SolveFailure, result.SolveFailure)
	}

	// Dump result projects into a map for easier interrogation
	rp := make(map[string]string)
	for _, p := range result.Projects {
		rp[string(p.Name)] = p.Version.Info
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

	return result
}
