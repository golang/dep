package vsolver

import (
	"fmt"
	"go/build"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/Masterminds/semver"
)

var cpath = path.Join(os.TempDir(), "smcache")
var bd string

type dummyAnalyzer struct{}

func (dummyAnalyzer) GetInfo(ctx build.Context, p ProjectName) (ProjectInfo, error) {
	return ProjectInfo{}, fmt.Errorf("just a dummy analyzer")
}

func init() {
	_, filename, _, _ := runtime.Caller(1)
	bd = path.Dir(filename)
}

func TestSourceManagerInit(t *testing.T) {
	// Just to ensure it's all clean
	os.RemoveAll(cpath)

	_, err := NewSourceManager(cpath, bd, true, false, dummyAnalyzer{})

	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
	}

	_, err = NewSourceManager(cpath, bd, true, false, dummyAnalyzer{})
	if err == nil {
		t.Errorf("Creating second SourceManager should have failed due to file lock contention")
	}

	sm, err := NewSourceManager(cpath, bd, true, true, dummyAnalyzer{})
	defer sm.Release()
	if err != nil {
		t.Errorf("Creating second SourceManager should have succeeded when force flag was passed, but failed with err %s", err)
	}

	if _, err = os.Stat(path.Join(cpath, "sm.lock")); err != nil {
		t.Errorf("Global cache lock file not created correctly")
	}
}

func TestProjectManagerInit(t *testing.T) {
	// Just to ensure it's all clean
	os.RemoveAll(cpath)
	sm, err := NewSourceManager(cpath, bd, true, false, dummyAnalyzer{})
	defer sm.Release()

	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
		t.FailNow()
	}

	pn := ProjectName("github.com/Masterminds/VCSTestRepo")
	v, err := sm.ListVersions(pn)
	if err != nil {
		t.Errorf("Unexpected error during initial project setup/fetching %s", err)
	}

	if len(v) != 3 {
		t.Errorf("Expected three version results from the test repo, got %v", len(v))
	} else {
		sv, _ := semver.NewVersion("1.0.0")
		rev := Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")
		expected := []Version{
			Version{
				Type:       V_Semver,
				Info:       "1.0.0",
				Underlying: rev,
				SemVer:     sv,
			},
			Version{
				Type:       V_Branch,
				Info:       "master",
				Underlying: rev,
			},
			Version{
				Type:       V_Branch,
				Info:       "test",
				Underlying: rev,
			},
		}

		for k, e := range expected {
			if v[k] != e {
				t.Errorf("Returned version in position %v had unexpected values:", v[k])
			}
		}
	}

	// Ensure that the appropriate cache dirs and files exist
	_, err = os.Stat(path.Join(cpath, "src", "github.com", "Masterminds", "VCSTestRepo", ".git"))
	if err != nil {
		t.Error("Cache repo does not exist in expected location")
	}

	_, err = os.Stat(path.Join(cpath, "metadata", "github.com", "Masterminds", "VCSTestRepo", "cache.json"))
	if err != nil {
		// TODO temporarily disabled until we turn caching back on
		//t.Error("Metadata cache json file does not exist in expected location")
	}

	// Ensure project existence values are what we expect
	var exists bool
	exists, err = sm.RepoExists(pn)
	if err != nil {
		t.Errorf("Error on checking RepoExists: %s", err)
	}
	if !exists {
		t.Error("Repo should exist after non-erroring call to ListVersions")
	}

	exists, err = sm.VendorCodeExists(pn)
	if err != nil {
		t.Errorf("Error on checking VendorCodeExists: %s", err)
	}
	if exists {
		t.Error("Shouldn't be any vendor code after just calling ListVersions")
	}

	// Now reach inside the black box
	pms, err := sm.(*sourceManager).getProjectManager(pn)
	if err != nil {
		t.Errorf("Error on grabbing project manager obj: %s", err)
	}

	// Check upstream existence flag
	if !pms.pm.CheckExistence(ExistsUpstream) {
		t.Errorf("ExistsUpstream flag not being correctly set the project")
	}
}
