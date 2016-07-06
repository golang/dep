package vsolver

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var fixtorun string

// TODO regression test ensuring that locks with only revs for projects don't cause errors
func init() {
	flag.StringVar(&fixtorun, "vsolver.fix", "", "A single fixture to run in TestBasicSolves")
}

var stderrlog = log.New(os.Stderr, "", 0)

func fixSolve(args SolveArgs, o SolveOpts, sm SourceManager) (Result, error) {
	if testing.Verbose() {
		o.Trace = true
		o.TraceLogger = stderrlog
	}

	si, err := Prepare(args, o, sm)
	s := si.(*solver)
	if err != nil {
		return nil, err
	}

	fixb := &depspecBridge{
		s.b.(*bridge),
	}
	s.b = fixb

	return s.Solve()
}

// Test all the basic table fixtures.
//
// Or, just the one named in the fix arg.
func TestBasicSolves(t *testing.T) {
	for _, fix := range basicFixtures {
		if fixtorun == "" || fixtorun == fix.n {
			solveBasicsAndCheck(fix, t)
			if testing.Verbose() {
				// insert a line break between tests
				stderrlog.Println("")
			}
		}
	}
}

func solveBasicsAndCheck(fix basicFixture, t *testing.T) (res Result, err error) {
	if testing.Verbose() {
		stderrlog.Printf("[[fixture %q]]", fix.n)
	}
	sm := newdepspecSM(fix.ds, nil)

	args := SolveArgs{
		Root:     string(fix.ds[0].Name()),
		Name:     ProjectName(fix.ds[0].Name()),
		Manifest: fix.ds[0],
		Lock:     dummyLock{},
	}

	o := SolveOpts{
		Downgrade: fix.downgrade,
		ChangeAll: fix.changeall,
	}

	if fix.l != nil {
		args.Lock = fix.l
	}

	res, err = fixSolve(args, o, sm)

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

func solveBimodalAndCheck(fix bimodalFixture, t *testing.T) (res Result, err error) {
	if testing.Verbose() {
		stderrlog.Printf("[[fixture %q]]", fix.n)
	}
	sm := newbmSM(fix)

	args := SolveArgs{
		Root:     string(fix.ds[0].Name()),
		Name:     ProjectName(fix.ds[0].Name()),
		Manifest: fix.ds[0],
		Lock:     dummyLock{},
		Ignore:   fix.ignore,
	}

	o := SolveOpts{
		Downgrade: fix.downgrade,
		ChangeAll: fix.changeall,
	}

	if fix.l != nil {
		args.Lock = fix.l
	}

	res, err = fixSolve(args, o, sm)

	return fixtureSolveSimpleChecks(fix, res, err, t)
}

func fixtureSolveSimpleChecks(fix specfix, res Result, err error, t *testing.T) (Result, error) {
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
			if errp[0] != string(fail.pn.LocalName) { // TODO identifierify
				t.Errorf("(fixture: %q) Expected failure on project %s, but was on project %s", fix.name(), errp[0], fail.pn.LocalName)
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
			for p, _ := range found {
				if _, has := ep[p]; !has {
					extra = append(extra, p)
				}
			}
			if len(extra) > 0 {
				t.Errorf("(fixture: %q) Expected solve failures due to projects %s, but solve failures also arose from %s", fix.name(), strings.Join(errp[1:], ", "), strings.Join(extra, ", "))
			}

			for p, _ := range ep {
				if _, has := found[p]; !has {
					missing = append(missing, p)
				}
			}
			if len(missing) > 0 {
				t.Errorf("(fixture: %q) Expected solve failures due to projects %s, but %s had no failures", fix.name(), strings.Join(errp[1:], ", "), strings.Join(missing, ", "))
			}

		default:
			// TODO round these out
			panic(fmt.Sprintf("unhandled solve failure type: %s", err))
		}
	} else if len(fix.expectErrs()) > 0 {
		t.Errorf("(fixture: %q) Solver succeeded, but expected failure", fix.name())
	} else {
		r := res.(result)
		if fix.maxTries() > 0 && r.Attempts() > fix.maxTries() {
			t.Errorf("(fixture: %q) Solver completed in %v attempts, but expected %v or fewer", fix.name(), r.att, fix.maxTries())
		}

		// Dump result projects into a map for easier interrogation
		rp := make(map[string]Version)
		for _, p := range r.p {
			pa := p.toAtom()
			rp[string(pa.id.LocalName)] = pa.v
		}

		fixlen, rlen := len(fix.result()), len(rp)
		if fixlen != rlen {
			// Different length, so they definitely disagree
			t.Errorf("(fixture: %q) Solver reported %v package results, result expected %v", fix.name(), rlen, fixlen)
		}

		// Whether or not len is same, still have to verify that results agree
		// Walk through fixture/expected results first
		for p, v := range fix.result() {
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
			if fv, exists := fix.result()[p]; !exists {
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
		r: mkresults(
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

	args := SolveArgs{
		Root:     string(fix.ds[0].Name()),
		Name:     ProjectName(fix.ds[0].Name()),
		Manifest: fix.ds[0],
		Lock:     l2,
	}

	res, err := fixSolve(args, SolveOpts{}, sm)

	fixtureSolveSimpleChecks(fix, res, err, t)
}

func getFailureCausingProjects(err error) (projs []string) {
	switch e := err.(type) {
	case *noVersionError:
		projs = append(projs, string(e.pn.LocalName)) // TODO identifierify
	case *disjointConstraintFailure:
		for _, f := range e.failsib {
			projs = append(projs, string(f.depender.id.LocalName))
		}
	case *versionNotAllowedFailure:
		for _, f := range e.failparent {
			projs = append(projs, string(f.depender.id.LocalName))
		}
	case *constraintNotAllowedFailure:
		// No sane way of knowing why the currently selected version is
		// selected, so do nothing
	case *sourceMismatchFailure:
		projs = append(projs, string(e.prob.id.LocalName))
		for _, c := range e.sel {
			projs = append(projs, string(c.depender.id.LocalName))
		}
	case *checkeeHasProblemPackagesFailure:
		projs = append(projs, string(e.goal.id.LocalName))
		for _, errdep := range e.failpkg {
			for _, atom := range errdep.deppers {
				projs = append(projs, string(atom.id.LocalName))
			}
		}
	case *depHasProblemPackagesFailure:
		projs = append(projs, string(e.goal.depender.id.LocalName), string(e.goal.dep.Ident.LocalName))
	case *nonexistentRevisionFailure:
		projs = append(projs, string(e.goal.depender.id.LocalName), string(e.goal.dep.Ident.LocalName))
	default:
		panic(fmt.Sprintf("unknown failtype %T, msg: %s", err, err))
	}

	return
}

func TestBadSolveOpts(t *testing.T) {
	sm := newdepspecSM(basicFixtures[0].ds, nil)

	o := SolveOpts{}
	args := SolveArgs{}
	_, err := Prepare(args, o, sm)
	if err == nil {
		t.Errorf("Should have errored on missing manifest")
	}

	m, _, _ := sm.GetProjectInfo(basicFixtures[0].ds[0].n, basicFixtures[0].ds[0].v)
	args.Manifest = m
	_, err = Prepare(args, o, sm)
	if err == nil {
		t.Errorf("Should have errored on empty root")
	}

	args.Root = "root"
	_, err = Prepare(args, o, sm)
	if err == nil {
		t.Errorf("Should have errored on empty name")
	}

	args.Name = "root"
	_, err = Prepare(args, o, sm)
	if err != nil {
		t.Errorf("Basic conditions satisfied, solve should have gone through, err was %s", err)
	}

	o.Trace = true
	_, err = Prepare(args, o, sm)
	if err == nil {
		t.Errorf("Should have errored on trace with no logger")
	}

	o.TraceLogger = log.New(ioutil.Discard, "", 0)
	_, err = Prepare(args, o, sm)
	if err != nil {
		t.Errorf("Basic conditions re-satisfied, solve should have gone through, err was %s", err)
	}
}

func TestIgnoreDedupe(t *testing.T) {
	fix := basicFixtures[0]

	ig := []string{"foo", "foo", "bar"}
	args := SolveArgs{
		Root:     string(fix.ds[0].Name()),
		Name:     ProjectName(fix.ds[0].Name()),
		Manifest: fix.ds[0],
		Ignore:   ig,
	}

	s, _ := Prepare(args, SolveOpts{}, newdepspecSM(basicFixtures[0].ds, nil))
	ts := s.(*solver)

	expect := map[string]bool{
		"foo": true,
		"bar": true,
	}

	if !reflect.DeepEqual(ts.ig, expect) {
		t.Errorf("Expected solver's ignore list to be deduplicated map, got %s", ts.ig)
	}
}
