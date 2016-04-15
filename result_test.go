package vsolver

import (
	"fmt"
	"go/build"
	"os"
	"path"
	"testing"
)

var basicResult Result
var kub ProjectAtom

// An analyzer that passes nothing back, but doesn't error. This expressly
// creates a situation that shouldn't be able to happen from a general solver
// perspective, so it's only useful for particular situations in tests
type passthruAnalyzer struct{}

func (passthruAnalyzer) GetInfo(ctx build.Context, p ProjectName) (ProjectInfo, error) {
	return ProjectInfo{}, nil
}

func init() {
	basicResult = Result{
		Attempts: 1,
		Projects: []ProjectAtom{
			ProjectAtom{
				Name:    "github.com/sdboyer/testrepo",
				Version: WithRevision(NewFloatingVersion("master"), Revision("4d59fb584b15a94d7401e356d2875c472d76ef45")),
			},
			ProjectAtom{
				Name:    "github.com/Masterminds/VCSTestRepo",
				Version: WithRevision(NewVersion("1.0.0"), Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")),
			},
		},
	}

	// just in case something needs punishing, kubernetes is happy to oblige
	kub = ProjectAtom{
		Name:    "github.com/kubernetes/kubernetes",
		Version: WithRevision(NewVersion("1.0.0"), Revision("528f879e7d3790ea4287687ef0ab3f2a01cc2718")),
	}
}

func TestResultCreateVendorTree(t *testing.T) {
	r := basicResult
	r.SolveFailure = fmt.Errorf("dummy error")

	tmp := path.Join(os.TempDir(), "vsolvtest")
	os.RemoveAll(tmp)

	sm, err := NewSourceManager(path.Join(tmp, "cache"), path.Join(tmp, "base"), true, false, passthruAnalyzer{})
	if err != nil {
		t.Errorf("NewSourceManager errored unexpectedly: %q", err)
	}

	err = r.CreateVendorTree(path.Join(tmp, "export"), sm)
	if err == fmt.Errorf("Cannot create vendor tree from failed solution. Failure was dummy error") {
		if err == nil {
			t.Errorf("Expected error due to result having solve failure, but no error")
		} else {
			t.Errorf("Expected error due to result having solve failure, but got %s", err)
		}
	}

	r.SolveFailure = nil
	err = r.CreateVendorTree(path.Join(tmp, "export"), sm)
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
	sm, err := NewSourceManager(path.Join(tmp, "cache"), path.Join(tmp, "base"), true, true, passthruAnalyzer{})
	if err != nil {
		b.Errorf("NewSourceManager errored unexpectedly: %q", err)
		clean = false
	}

	// Prefetch the projects before timer starts
	for _, pa := range r.Projects {
		_, err := sm.GetProjectInfo(pa)
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
			err = r.CreateVendorTree(exp, sm)
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
