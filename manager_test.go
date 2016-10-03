package gps

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
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

	sm, err := NewSourceManager(naiveAnalyzer{}, cpath)
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
	sm, err := NewSourceManager(naiveAnalyzer{}, cpath)

	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
	}

	_, err = NewSourceManager(naiveAnalyzer{}, cpath)
	if err == nil {
		t.Errorf("Creating second SourceManager should have failed due to file lock contention")
	} else if te, ok := err.(CouldNotCreateLockError); !ok {
		t.Errorf("Should have gotten CouldNotCreateLockError error type, but got %T", te)
	}

	if _, err = os.Stat(path.Join(cpath, "sm.lock")); err != nil {
		t.Errorf("Global cache lock file not created correctly")
	}

	sm.Release()
	err = removeAll(cpath)
	if err != nil {
		t.Errorf("removeAll failed: %s", err)
	}

	if _, err = os.Stat(path.Join(cpath, "sm.lock")); !os.IsNotExist(err) {
		t.Errorf("Global cache lock file not cleared correctly on Release()")
		t.FailNow()
	}

	// Set another one up at the same spot now, just to be sure
	sm, err = NewSourceManager(naiveAnalyzer{}, cpath)
	if err != nil {
		t.Errorf("Creating a second SourceManager should have succeeded when the first was released, but failed with err %s", err)
	}

	sm.Release()
	err = removeAll(cpath)
	if err != nil {
		t.Errorf("removeAll failed: %s", err)
	}
}

func TestSourceInit(t *testing.T) {
	// This test is a bit slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping project manager init test in short mode")
	}

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
		t.FailNow()
	}

	sm, err := NewSourceManager(naiveAnalyzer{}, cpath)
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

	id := mkPI("github.com/Masterminds/VCSTestRepo").normalize()
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
		SortForUpgrade(v)

		for k, e := range expected {
			if !v[k].Matches(e) {
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

	rev := Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")
	if len(v) != 3 {
		t.Errorf("Expected three version results from the test repo, got %v", len(v))
	} else {
		expected := []Version{
			NewVersion("1.0.0").Is(rev),
			NewBranch("master").Is(rev),
			NewBranch("test").Is(rev),
		}

		for k, e := range expected {
			if !v[k].Matches(e) {
				t.Errorf("Expected version %s in position %v but got %s", e, k, v[k])
			}
		}

		if !v[1].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected master branch version to have isDefault flag, but it did not")
		}
		if v[2].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected test branch version not to have isDefault flag, but it did")
		}
	}

	present, err := smc.RevisionPresentIn(id, rev)
	if err != nil {
		t.Errorf("Should have found revision in source, but got err: %s", err)
	} else if !present {
		t.Errorf("Should have found revision in source, but did not")
	}

	// SyncSourceFor will ensure we have everything
	err = smc.SyncSourceFor(id)
	if err != nil {
		t.Errorf("SyncSourceFor failed with unexpected error: %s", err)
	}

	// Ensure that the appropriate cache dirs and files exist
	_, err = os.Stat(filepath.Join(cpath, "sources", "https---github.com-Masterminds-VCSTestRepo", ".git"))
	if err != nil {
		t.Error("Cache repo does not exist in expected location")
	}

	_, err = os.Stat(filepath.Join(cpath, "metadata", "github.com", "Masterminds", "VCSTestRepo", "cache.json"))
	if err != nil {
		// TODO(sdboyer) disabled until we get caching working
		//t.Error("Metadata cache json file does not exist in expected location")
	}

	// Ensure source existence values are what we expect
	var exists bool
	exists, err = sm.SourceExists(id)
	if err != nil {
		t.Errorf("Error on checking SourceExists: %s", err)
	}
	if !exists {
		t.Error("Source should exist after non-erroring call to ListVersions")
	}
}

func TestDefaultBranchAssignment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping default branch assignment test in short mode")
	}

	sm, clean := mkNaiveSM(t)
	defer clean()

	id := mkPI("github.com/sdboyer/test-multibranch")
	v, err := sm.ListVersions(id)
	if err != nil {
		t.Errorf("Unexpected error during initial project setup/fetching %s", err)
	}

	if len(v) != 3 {
		t.Errorf("Expected three version results from the test repo, got %v", len(v))
	} else {
		brev := Revision("fda020843ac81352004b9dca3fcccdd517600149")
		mrev := Revision("9f9c3a591773d9b28128309ac7a9a72abcab267d")
		expected := []Version{
			NewBranch("branchone").Is(brev),
			NewBranch("otherbranch").Is(brev),
			NewBranch("master").Is(mrev),
		}

		SortForUpgrade(v)

		for k, e := range expected {
			if !v[k].Matches(e) {
				t.Errorf("Expected version %s in position %v but got %s", e, k, v[k])
			}
		}

		if !v[0].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected branchone branch version to have isDefault flag, but it did not")
		}
		if !v[0].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected otherbranch branch version to have isDefault flag, but it did not")
		}
		if v[2].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected master branch version not to have isDefault flag, but it did")
		}
	}
}

func TestMgrMethodsFailWithBadPath(t *testing.T) {
	// a symbol will always bork it up
	bad := mkPI("foo/##&^").normalize()
	sm, clean := mkNaiveSM(t)
	defer clean()

	var err error
	if _, err = sm.SourceExists(bad); err == nil {
		t.Error("SourceExists() did not error on bad input")
	}
	if err = sm.SyncSourceFor(bad); err == nil {
		t.Error("SyncSourceFor() did not error on bad input")
	}
	if _, err = sm.ListVersions(bad); err == nil {
		t.Error("ListVersions() did not error on bad input")
	}
	if _, err = sm.RevisionPresentIn(bad, Revision("")); err == nil {
		t.Error("RevisionPresentIn() did not error on bad input")
	}
	if _, err = sm.ListPackages(bad, nil); err == nil {
		t.Error("ListPackages() did not error on bad input")
	}
	if _, _, err = sm.GetManifestAndLock(bad, nil); err == nil {
		t.Error("GetManifestAndLock() did not error on bad input")
	}
	if err = sm.ExportProject(bad, nil, ""); err == nil {
		t.Error("ExportProject() did not error on bad input")
	}
}

func TestGetSources(t *testing.T) {
	// This test is a tad slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping source setup test in short mode")
	}

	sm, clean := mkNaiveSM(t)

	pil := []ProjectIdentifier{
		mkPI("github.com/Masterminds/VCSTestRepo").normalize(),
		mkPI("bitbucket.org/mattfarina/testhgrepo").normalize(),
		mkPI("launchpad.net/govcstestbzrrepo").normalize(),
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)
	for _, pi := range pil {
		go func(lpi ProjectIdentifier) {
			nn := lpi.netName()
			src, err := sm.getSourceFor(lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error setting up source: %s", nn, err)
				return
			}

			// Re-get the same, make sure they are the same
			src2, err := sm.getSourceFor(lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error re-getting source: %s", nn, err)
			} else if src != src2 {
				t.Errorf("(src %q) first and second sources are not eq", nn)
			}

			// All of them _should_ select https, so this should work
			lpi.NetworkName = "https://" + lpi.NetworkName
			src3, err := sm.getSourceFor(lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error getting explicit https source: %s", nn, err)
			} else if src != src3 {
				t.Errorf("(src %q) explicit https source should reuse autodetected https source", nn)
			}

			// Now put in http, and they should differ
			lpi.NetworkName = "http://" + string(lpi.ProjectRoot)
			src4, err := sm.getSourceFor(lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error getting explicit http source: %s", nn, err)
			} else if src == src4 {
				t.Errorf("(src %q) explicit http source should create a new src", nn)
			}

			wg.Done()
		}(pi)
	}

	wg.Wait()

	// nine entries (of which three are dupes): for each vcs, raw import path,
	// the https url, and the http url
	if len(sm.srcs) != 9 {
		t.Errorf("Should have nine discrete entries in the srcs map, got %v", len(sm.srcs))
	}
	clean()
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

	id := mkPI("github.com/Masterminds/VCSTestRepo").normalize()

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
		t.Errorf("Root path trie should still have one element after performing the same deduction twice; has %v", sm.rootxt.Len())
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

	// Ensure that vcs extension-based matching comes through
	in5 := "ffffrrrraaaaaapppppdoesnotresolve.com/baz.git"
	pr, err = sm.DeduceProjectRoot(in5)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", in5, err)
	} else if string(pr) != in5 {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.rootxt.Len() != 6 {
		t.Errorf("Root path trie should have six elements, one for each unique root and subpath; has %v", sm.rootxt.Len())
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
		defer func() {
			wg.Done()
			if e := recover(); e != nil {
				t.Errorf("goroutine number %v panicked with err: %s", rnum, e)
			}
		}()
		<-c
		_, err := rootf()
		if err != nil {
			t.Errorf("err was non-nil on root detection in goroutine number %v: %s", rnum, err)
		}
	}

	for k := range make([]struct{}, cnum) {
		wg.Add(1)
		go f(k)
		runtime.Gosched()
	}
	close(c)
	wg.Wait()
	if sm.rootxt.Len() != 1 {
		t.Errorf("Root path trie should have just one element; has %v", sm.rootxt.Len())
	}

	// repeat for srcf
	wg2 := &sync.WaitGroup{}
	c = make(chan struct{}, 1)
	f = func(rnum int) {
		defer func() {
			wg2.Done()
			if e := recover(); e != nil {
				t.Errorf("goroutine number %v panicked with err: %s", rnum, e)
			}
		}()
		<-c
		_, _, err := srcf()
		if err != nil {
			t.Errorf("err was non-nil on root detection in goroutine number %v: %s", rnum, err)
		}
	}

	for k := range make([]struct{}, cnum) {
		wg2.Add(1)
		go f(k)
		runtime.Gosched()
	}
	close(c)
	wg2.Wait()
	if len(sm.srcs) != 2 {
		t.Errorf("Sources map should have just two elements, but has %v", len(sm.srcs))
	}
}
