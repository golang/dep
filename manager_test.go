package gps

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/Masterminds/semver"
)

var bd string

// An analyzer that passes nothing back, but doesn't error. This is the naive
// case - no constraints, no lock, and no errors. The SourceMgr will interpret
// this as open/Any constraints on everything in the import graph.
type naiveAnalyzer struct{}

func (naiveAnalyzer) DeriveManifestAndLock(string, ProjectRoot) (Manifest, Lock, error) {
	return nil, nil, nil
}

func (a naiveAnalyzer) Info() (name string, version *semver.Version) {
	return "naive-analyzer", sv("v0.0.1")
}

func sv(s string) *semver.Version {
	sv, err := semver.NewVersion(s)
	if err != nil {
		panic(fmt.Sprintf("Error creating semver from %q: %s", s, err))
	}

	return sv
}

func mkNaiveSM(t *testing.T) (*SourceMgr, func()) {
	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
		t.FailNow()
	}

	sm, err := NewSourceManager(naiveAnalyzer{}, cpath, false)
	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
		t.FailNow()
	}

	return sm, func() {
		sm.Release()
		err := removeAll(cpath)
		if err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}
}

func init() {
	_, filename, _, _ := runtime.Caller(1)
	bd = path.Dir(filename)
}

func TestSourceManagerInit(t *testing.T) {
	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
	}
	_, err = NewSourceManager(naiveAnalyzer{}, cpath, false)

	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
	}
	defer func() {
		err := removeAll(cpath)
		if err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}()

	_, err = NewSourceManager(naiveAnalyzer{}, cpath, false)
	if err == nil {
		t.Errorf("Creating second SourceManager should have failed due to file lock contention")
	}

	sm, err := NewSourceManager(naiveAnalyzer{}, cpath, true)
	defer sm.Release()
	if err != nil {
		t.Errorf("Creating second SourceManager should have succeeded when force flag was passed, but failed with err %s", err)
	}

	if _, err = os.Stat(path.Join(cpath, "sm.lock")); err != nil {
		t.Errorf("Global cache lock file not created correctly")
	}
}

func TestProjectManagerInit(t *testing.T) {
	// This test is a bit slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping project manager init test in short mode")
	}

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
		t.FailNow()
	}

	sm, err := NewSourceManager(naiveAnalyzer{}, cpath, false)
	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
		t.FailNow()
	}

	defer func() {
		sm.Release()
		err := removeAll(cpath)
		if err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}()

	id := mkPI("github.com/Masterminds/VCSTestRepo")
	v, err := sm.ListVersions(id)
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
	// works (or at least doesn't not work) by asking for the versions again,
	// and do it through smcache to ensure its sorting works, as well.
	smc := &bridge{
		sm:     sm,
		vlists: make(map[ProjectIdentifier][]Version),
		s:      &solver{},
	}

	v, err = smc.ListVersions(id)
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

		for k, e := range expected {
			if v[k] != e {
				t.Errorf("Expected version %s in position %v but got %s", e, k, v[k])
			}
		}
	}

	// Ensure that the appropriate cache dirs and files exist
	_, err = os.Stat(path.Join(cpath, "sources", "https---github.com-Masterminds-VCSTestRepo", ".git"))
	if err != nil {
		t.Error("Cache repo does not exist in expected location")
	}

	_, err = os.Stat(path.Join(cpath, "metadata", "github.com", "Masterminds", "VCSTestRepo", "cache.json"))
	if err != nil {
		// TODO(sdboyer) disabled until we get caching working
		//t.Error("Metadata cache json file does not exist in expected location")
	}

	// Ensure project existence values are what we expect
	var exists bool
	exists, err = sm.SourceExists(id)
	if err != nil {
		t.Errorf("Error on checking SourceExists: %s", err)
	}
	if !exists {
		t.Error("Repo should exist after non-erroring call to ListVersions")
	}

	// Now reach inside the black box
	pms, err := sm.getProjectManager(id)
	if err != nil {
		t.Errorf("Error on grabbing project manager obj: %s", err)
		t.FailNow()
	}

	// Check upstream existence flag
	if !pms.pm.CheckExistence(existsUpstream) {
		t.Errorf("ExistsUpstream flag not being correctly set the project")
	}
}

func TestRepoVersionFetching(t *testing.T) {
	// This test is quite slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping repo version fetching test in short mode")
	}

	sm, clean := mkNaiveSM(t)

	upstreams := []ProjectIdentifier{
		mkPI("github.com/Masterminds/VCSTestRepo"),
		mkPI("bitbucket.org/mattfarina/testhgrepo"),
		mkPI("launchpad.net/govcstestbzrrepo"),
	}

	pms := make([]*projectManager, len(upstreams))
	for k, u := range upstreams {
		pmi, err := sm.getProjectManager(u)
		if err != nil {
			clean()
			t.Errorf("Unexpected error on ProjectManager creation: %s", err)
			t.FailNow()
		}
		pms[k] = pmi.pm
	}

	defer clean()

	// test git first
	vlist, exbits, err := pms[0].crepo.getCurrentVersionPairs()
	if err != nil {
		t.Errorf("Unexpected error getting version pairs from git repo: %s", err)
	}
	if exbits != existsUpstream {
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
	if exbits != existsUpstream|existsInCache {
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
	if exbits != existsUpstream|existsInCache {
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

// Regression test for #32
func TestGetInfoListVersionsOrdering(t *testing.T) {
	// This test is quite slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping slow test in short mode")
	}

	sm, clean := mkNaiveSM(t)
	defer clean()

	// setup done, now do the test

	id := mkPI("github.com/Masterminds/VCSTestRepo")

	_, _, err := sm.GetManifestAndLock(id, NewVersion("1.0.0"))
	if err != nil {
		t.Errorf("Unexpected error from GetInfoAt %s", err)
	}

	v, err := sm.ListVersions(id)
	if err != nil {
		t.Errorf("Unexpected error from ListVersions %s", err)
	}

	if len(v) != 3 {
		t.Errorf("Expected three results from ListVersions, got %v", len(v))
	}
}

func TestDeduceProjectRoot(t *testing.T) {
	sm, clean := mkNaiveSM(t)
	defer clean()

	in := "github.com/sdboyer/gps"
	pr, err := sm.DeduceProjectRoot(in)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", in, err)
	}
	if string(pr) != in {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.rootxt.Len() != 1 {
		t.Errorf("Root path trie should have one element after one deduction, has %v", sm.rootxt.Len())
	}

	pr, err = sm.DeduceProjectRoot(in)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", in, err)
	} else if string(pr) != in {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.rootxt.Len() != 1 {
		t.Errorf("Root path trie should have one element after performing the same deduction twice; has %v", sm.rootxt.Len())
	}

	// Now do a subpath
	sub := path.Join(in, "foo")
	pr, err = sm.DeduceProjectRoot(sub)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", sub, err)
	} else if string(pr) != in {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.rootxt.Len() != 2 {
		t.Errorf("Root path trie should have two elements, one for root and one for subpath; has %v", sm.rootxt.Len())
	}

	// Now do a fully different root, but still on github
	in2 := "github.com/bagel/lox"
	sub2 := path.Join(in2, "cheese")
	pr, err = sm.DeduceProjectRoot(sub2)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", sub2, err)
	} else if string(pr) != in2 {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.rootxt.Len() != 4 {
		t.Errorf("Root path trie should have four elements, one for each unique root and subpath; has %v", sm.rootxt.Len())
	}

	// Ensure that our prefixes are bounded by path separators
	in4 := "github.com/bagel/loxx"
	pr, err = sm.DeduceProjectRoot(in4)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", in4, err)
	} else if string(pr) != in4 {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.rootxt.Len() != 5 {
		t.Errorf("Root path trie should have five elements, one for each unique root and subpath; has %v", sm.rootxt.Len())
	}
}

// Test that the future returned from SourceMgr.deducePathAndProcess() is safe
// to call concurrently.
//
// Obviously, this is just a heuristic; passage does not guarantee correctness
// (though failure does guarantee incorrectness)
func TestMultiDeduceThreadsafe(t *testing.T) {
	sm, clean := mkNaiveSM(t)
	defer clean()

	in := "github.com/sdboyer/gps"
	rootf, srcf, err := sm.deducePathAndProcess(in)
	if err != nil {
		t.Errorf("Known-good path %q had unexpected basic deduction error: %s", in, err)
		t.FailNow()
	}

	cnum := 50
	wg := &sync.WaitGroup{}

	// Set up channel for everything else to block on
	c := make(chan struct{}, 1)
	f := func(rnum int) {
		wg.Add(1)
		defer func() {
			if e := recover(); e != nil {
				t.Errorf("goroutine number %v panicked with err: %s", rnum, e)
			}
		}()
		<-c
		_, err := rootf()
		if err != nil {
			t.Errorf("err was non-nil on root detection in goroutine number %v: %s", rnum, err)
		}
		wg.Done()
	}

	for k := range make([]struct{}, cnum) {
		go f(k)
		runtime.Gosched()
	}
	close(c)
	wg.Wait()
	if sm.rootxt.Len() != 1 {
		t.Errorf("Root path trie should have just one element; has %v", sm.rootxt.Len())
	}

	// repeat for srcf
	c = make(chan struct{}, 1)
	f = func(rnum int) {
		wg.Add(1)
		defer func() {
			if e := recover(); e != nil {
				t.Errorf("goroutine number %v panicked with err: %s", rnum, e)
			}
		}()
		<-c
		_, _, err := srcf()
		if err != nil {
			t.Errorf("err was non-nil on root detection in goroutine number %v: %s", rnum, err)
		}
		wg.Done()
	}

	for k := range make([]struct{}, cnum) {
		go f(k)
		runtime.Gosched()
	}
	close(c)
	wg.Wait()
	if len(sm.srcs) != 2 {
		t.Errorf("Sources map should have just two elements, but has %v", len(sm.srcs))
	}
}
