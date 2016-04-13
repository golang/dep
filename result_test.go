package vsolver

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/Masterminds/semver"
)

var basicResult Result

func init() {
	sv1, _ := semver.NewVersion("1.0.0")
	basicResult = Result{
		Attempts: 1,
		Projects: []ProjectAtom{
			ProjectAtom{
				Name: "github.com/sdboyer/testrepo",
				Version: Version{
					Type:       V_Branch,
					Info:       "master",
					Underlying: "4d59fb584b15a94d7401e356d2875c472d76ef45",
				},
			},
			ProjectAtom{
				Name: "github.com/Masterminds/VCSTestRepo",
				Version: Version{
					Type:       V_Semver,
					Info:       "1.0.0",
					Underlying: "30605f6ac35fcb075ad0bfa9296f90a7d891523e",
					SemVer:     sv1,
				},
			},
		},
	}
}

func TestResultCreateVendorTree(t *testing.T) {
	r := basicResult
	r.SolveFailure = fmt.Errorf("dummy error")

	tmp := path.Join(os.TempDir(), "vsolvtest")
	os.RemoveAll(tmp)
	//fmt.Println(tmp)

	sm, err := NewSourceManager(path.Join(tmp, "cache"), path.Join(tmp, "base"), true, false, dummyAnalyzer{})
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
