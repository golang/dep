package vsolver

import (
	"fmt"
	"go/build"
	"os"
	"path"
	"runtime"
	"sort"
	"testing"

	"github.com/Masterminds/semver"
)

var cpath = path.Join(os.TempDir(), "smcache")
var bd string

type dummyAnalyzer struct{}

func (dummyAnalyzer) GetInfo(ctx build.Context, p ProjectName) (Manifest, Lock, error) {
	return nil, nil, fmt.Errorf("just a dummy analyzer")
}

func sv(s string) *semver.Version {
	sv, err := semver.NewVersion(s)
	if err != nil {
		panic(fmt.Sprintf("Error creating semver from %q: %s", s, err))
	}

	return sv
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

	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
		t.FailNow()
	}
	defer sm.Release()

	pn := ProjectName("github.com/Masterminds/VCSTestRepo")
	v, err := sm.ListVersions(pn)
	if err != nil {
		t.Errorf("Unexpected error during initial project setup/fetching %s", err)
	}

	if len(v) != 3 {
		t.Errorf("Expected three version results from the test repo, got %v", len(v))
	} else {
		rev := Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")
		expected := []Version{
			NewVersion("1.0.0").Is(rev),
			NewBranch("master").Is(rev),
			NewBranch("test").Is(rev),
		}

		// SourceManager itself doesn't guarantee ordering; sort them here so we
		// can dependably check output
		sort.Sort(upgradeVersionSorter(v))

		for k, e := range expected {
			if v[k] != e {
				t.Errorf("Expected version %s in position %v but got %s", e, k, v[k])
			}
		}
	}

	// Two birds, one stone - make sure the internal ProjectManager vlist cache
	// works by asking for the versions again, and do it through smcache to
	// ensure its sorting works, as well.

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

func TestRepoVersionFetching(t *testing.T) {
	os.RemoveAll(cpath)
	smi, err := NewSourceManager(cpath, bd, true, false, dummyAnalyzer{})
	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
		t.FailNow()
	}

	sm := smi.(*sourceManager)
	upstreams := []ProjectName{
		"github.com/Masterminds/VCSTestRepo",
		"bitbucket.org/mattfarina/testhgrepo",
		"launchpad.net/govcstestbzrrepo",
	}

	pms := make([]*projectManager, len(upstreams))
	for k, u := range upstreams {
		pmi, err := sm.getProjectManager(u)
		if err != nil {
			sm.Release()
			t.Errorf("Unexpected error on ProjectManager creation: %s", err)
			t.FailNow()
		}
		pms[k] = pmi.pm.(*projectManager)
	}

	defer sm.Release()

	// test git first
	vlist, exbits, err := pms[0].crepo.getCurrentVersionPairs()
	if err != nil {
		t.Errorf("Unexpected error getting version pairs from git repo: %s", err)
	}
	if exbits != ExistsUpstream {
		t.Errorf("git pair fetch should only set upstream existence bits, but got %v", exbits)
	}
	if len(vlist) != 3 {
		t.Errorf("git test repo should've produced three versions, got %v", len(vlist))
	} else {
		v := NewBranch("master").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e"))
		if vlist[0] != v {
			t.Errorf("git pair fetch reported incorrect first version, got %s", vlist[0])
		}

		v = NewBranch("test").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e"))
		if vlist[1] != v {
			t.Errorf("git pair fetch reported incorrect second version, got %s", vlist[1])
		}

		v = NewVersion("1.0.0").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e"))
		if vlist[2] != v {
			t.Errorf("git pair fetch reported incorrect third version, got %s", vlist[2])
		}
	}

	// now hg
	vlist, exbits, err = pms[1].crepo.getCurrentVersionPairs()
	if err != nil {
		t.Errorf("Unexpected error getting version pairs from hg repo: %s", err)
	}
	if exbits != ExistsUpstream|ExistsInCache {
		t.Errorf("hg pair fetch should set upstream and cache existence bits, but got %v", exbits)
	}
	if len(vlist) != 2 {
		t.Errorf("hg test repo should've produced two versions, got %v", len(vlist))
	} else {
		v := NewVersion("1.0.0").Is(Revision("d680e82228d206935ab2eaa88612587abe68db07"))
		if vlist[0] != v {
			t.Errorf("hg pair fetch reported incorrect first version, got %s", vlist[0])
		}

		v = NewBranch("test").Is(Revision("6c44ee3fe5d87763616c19bf7dbcadb24ff5a5ce"))
		if vlist[1] != v {
			t.Errorf("hg pair fetch reported incorrect second version, got %s", vlist[1])
		}
	}

	// bzr last
	vlist, exbits, err = pms[2].crepo.getCurrentVersionPairs()
	if err != nil {
		t.Errorf("Unexpected error getting version pairs from bzr repo: %s", err)
	}
	if exbits != ExistsUpstream|ExistsInCache {
		t.Errorf("bzr pair fetch should set upstream and cache existence bits, but got %v", exbits)
	}
	if len(vlist) != 1 {
		t.Errorf("bzr test repo should've produced one version, got %v", len(vlist))
	} else {
		v := NewVersion("1.0.0").Is(Revision("matt@mattfarina.com-20150731135137-pbphasfppmygpl68"))
		if vlist[0] != v {
			t.Errorf("bzr pair fetch reported incorrect first version, got %s", vlist[0])
		}
	}
	// no svn for now, because...svn
}
