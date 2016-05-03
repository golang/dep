package vsolver

import (
	"go/build"
	"os"
	"path"
	"testing"
)

var basicResult result
var kub ProjectAtom

// An analyzer that passes nothing back, but doesn't error. This expressly
// creates a situation that shouldn't be able to happen from a general solver
// perspective, so it's only useful for particular situations in tests
type passthruAnalyzer struct{}

func (passthruAnalyzer) GetInfo(ctx build.Context, p ProjectName) (Manifest, Lock, error) {
	return nil, nil, nil
}

func pi(n string) ProjectIdentifier {
	return ProjectIdentifier{
		LocalName: ProjectName(n),
	}
}

func init() {
	basicResult = result{
		att: 1,
		p: []LockedProject{
			pa2lp(ProjectAtom{
				Name:    pi("github.com/sdboyer/testrepo"),
				Version: NewBranch("master").Is(Revision("4d59fb584b15a94d7401e356d2875c472d76ef45")),
			}),
			pa2lp(ProjectAtom{
				Name:    pi("github.com/Masterminds/VCSTestRepo"),
				Version: NewVersion("1.0.0").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")),
			}),
		},
	}

	// just in case something needs punishing, kubernetes is happy to oblige
	kub = ProjectAtom{
		Name:    pi("github.com/kubernetes/kubernetes"),
		Version: NewVersion("1.0.0").Is(Revision("528f879e7d3790ea4287687ef0ab3f2a01cc2718")),
	}
}

func TestResultCreateVendorTree(t *testing.T) {
	r := basicResult

	tmp := path.Join(os.TempDir(), "vsolvtest")
	os.RemoveAll(tmp)

	sm, err := NewSourceManager(path.Join(tmp, "cache"), path.Join(tmp, "base"), false, passthruAnalyzer{})
	if err != nil {
		t.Errorf("NewSourceManager errored unexpectedly: %q", err)
	}

	err = CreateVendorTree(path.Join(tmp, "export"), r, sm)
	if err != nil {
		t.Errorf("Unexpected error while creating vendor tree: %s", err)
	}

	// TODO add more checks
}

func BenchmarkCreateVendorTree(b *testing.B) {
	// We're fs-bound here, so restrict to single parallelism
	b.SetParallelism(1)

	r := basicResult
	tmp := path.Join(os.TempDir(), "vsolvtest")

	clean := true
	sm, err := NewSourceManager(path.Join(tmp, "cache"), path.Join(tmp, "base"), true, passthruAnalyzer{})
	if err != nil {
		b.Errorf("NewSourceManager errored unexpectedly: %q", err)
		clean = false
	}

	// Prefetch the projects before timer starts
	for _, lp := range r.p {
		_, err := sm.GetProjectInfo(lp.n, lp.Version())
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
			err = CreateVendorTree(exp, r, sm)
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
