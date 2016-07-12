package vsolver

import (
	"os"
	"path"
	"testing"
)

var basicResult solution
var kub atom

func pi(n string) ProjectIdentifier {
	return ProjectIdentifier{
		ProjectRoot: ProjectRoot(n),
	}
}

func init() {
	basicResult = solution{
		att: 1,
		p: []LockedProject{
			pa2lp(atom{
				id: pi("github.com/sdboyer/testrepo"),
				v:  NewBranch("master").Is(Revision("4d59fb584b15a94d7401e356d2875c472d76ef45")),
			}, nil),
			pa2lp(atom{
				id: pi("github.com/Masterminds/VCSTestRepo"),
				v:  NewVersion("1.0.0").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")),
			}, nil),
		},
	}

	// just in case something needs punishing, kubernetes is happy to oblige
	kub = atom{
		id: pi("github.com/kubernetes/kubernetes"),
		v:  NewVersion("1.0.0").Is(Revision("528f879e7d3790ea4287687ef0ab3f2a01cc2718")),
	}
}

func TestResultCreateVendorTree(t *testing.T) {
	// This test is a bit slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping vendor tree creation test in short mode")
	}

	r := basicResult

	tmp := path.Join(os.TempDir(), "vsolvtest")
	os.RemoveAll(tmp)

	sm, err := NewSourceManager(naiveAnalyzer{}, path.Join(tmp, "cache"), false)
	if err != nil {
		t.Errorf("NewSourceManager errored unexpectedly: %q", err)
	}

	err = CreateVendorTree(path.Join(tmp, "export"), r, sm, true)
	if err != nil {
		t.Errorf("Unexpected error while creating vendor tree: %s", err)
	}

	// TODO(sdboyer) add more checks
}

func BenchmarkCreateVendorTree(b *testing.B) {
	// We're fs-bound here, so restrict to single parallelism
	b.SetParallelism(1)

	r := basicResult
	tmp := path.Join(os.TempDir(), "vsolvtest")

	clean := true
	sm, err := NewSourceManager(naiveAnalyzer{}, path.Join(tmp, "cache"), true)
	if err != nil {
		b.Errorf("NewSourceManager errored unexpectedly: %q", err)
		clean = false
	}

	// Prefetch the projects before timer starts
	for _, lp := range r.p {
		_, _, err := sm.GetProjectInfo(lp.Ident().ProjectRoot, lp.Version())
		if err != nil {
			b.Errorf("failed getting project info during prefetch: %s", err)
			clean = false
		}
	}

	if clean {
		b.ResetTimer()
		b.StopTimer()
		exp := path.Join(tmp, "export")
		for i := 0; i < b.N; i++ {
			// Order the loop this way to make it easy to disable final cleanup, to
			// ease manual inspection
			os.RemoveAll(exp)
			b.StartTimer()
			err = CreateVendorTree(exp, r, sm, true)
			b.StopTimer()
			if err != nil {
				b.Errorf("unexpected error after %v iterations: %s", i, err)
				break
			}
		}
	}

	sm.Release()
	os.RemoveAll(tmp) // comment this to leave temp dir behind for inspection
}
