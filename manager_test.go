package gps

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func (a naiveAnalyzer) Info() (name string, version int) {
	return "naive-analyzer", 1
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

func remakeNaiveSM(osm *SourceMgr, t *testing.T) (*SourceMgr, func()) {
	cpath := osm.cachedir
	osm.Release()

	sm, err := NewSourceManager(naiveAnalyzer{}, cpath)
	if err != nil {
		t.Errorf("unexpected error on SourceManager recreation: %s", err)
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

	id := mkPI("github.com/sdboyer/gpkt").normalize()
	v, err := sm.ListVersions(id)
	if err != nil {
		t.Errorf("Unexpected error during initial project setup/fetching %s", err)
	}

	if len(v) != 7 {
		t.Errorf("Expected seven version results from the test repo, got %v", len(v))
	} else {
		expected := []Version{
			NewVersion("v2.0.0").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
			NewVersion("v1.1.0").Is(Revision("b2cb48dda625f6640b34d9ffb664533359ac8b91")),
			NewVersion("v1.0.0").Is(Revision("bf85021c0405edbc4f3648b0603818d641674f72")),
			newDefaultBranch("master").Is(Revision("bf85021c0405edbc4f3648b0603818d641674f72")),
			NewBranch("v1").Is(Revision("e3777f683305eafca223aefe56b4e8ecf103f467")),
			NewBranch("v1.1").Is(Revision("f1fbc520489a98306eb28c235204e39fa8a89c84")),
			NewBranch("v3").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
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
		s:      &solver{mtr: newMetrics()},
	}

	v, err = smc.ListVersions(id)
	if err != nil {
		t.Errorf("Unexpected error during initial project setup/fetching %s", err)
	}

	if len(v) != 7 {
		t.Errorf("Expected seven version results from the test repo, got %v", len(v))
	} else {
		expected := []Version{
			NewVersion("v2.0.0").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
			NewVersion("v1.1.0").Is(Revision("b2cb48dda625f6640b34d9ffb664533359ac8b91")),
			NewVersion("v1.0.0").Is(Revision("bf85021c0405edbc4f3648b0603818d641674f72")),
			newDefaultBranch("master").Is(Revision("bf85021c0405edbc4f3648b0603818d641674f72")),
			NewBranch("v1").Is(Revision("e3777f683305eafca223aefe56b4e8ecf103f467")),
			NewBranch("v1.1").Is(Revision("f1fbc520489a98306eb28c235204e39fa8a89c84")),
			NewBranch("v3").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
		}

		for k, e := range expected {
			if !v[k].Matches(e) {
				t.Errorf("Expected version %s in position %v but got %s", e, k, v[k])
			}
		}

		if !v[3].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected master branch version to have isDefault flag, but it did not")
		}
		if v[4].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected v1 branch version not to have isDefault flag, but it did")
		}
		if v[5].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected v1.1 branch version not to have isDefault flag, but it did")
		}
		if v[6].(versionPair).v.(branchVersion).isDefault {
			t.Error("Expected v3 branch version not to have isDefault flag, but it did")
		}
	}

	present, err := smc.RevisionPresentIn(id, Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e"))
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
	_, err = os.Stat(filepath.Join(cpath, "sources", "https---github.com-sdboyer-gpkt", ".git"))
	if err != nil {
		t.Error("Cache repo does not exist in expected location")
	}

	_, err = os.Stat(filepath.Join(cpath, "metadata", "github.com", "sdboyer", "gpkt", "cache.json"))
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
	requiresBins(t, "git", "hg", "bzr")

	sm, clean := mkNaiveSM(t)

	pil := []ProjectIdentifier{
		mkPI("github.com/Masterminds/VCSTestRepo").normalize(),
		mkPI("bitbucket.org/mattfarina/testhgrepo").normalize(),
		mkPI("launchpad.net/govcstestbzrrepo").normalize(),
	}

	ctx := context.Background()
	wg := &sync.WaitGroup{}
	wg.Add(3)
	for _, pi := range pil {
		go func(lpi ProjectIdentifier) {
			defer wg.Done()

			nn := lpi.normalizedSource()
			srcg, err := sm.srcCoord.getSourceGatewayFor(ctx, lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error setting up source: %s", nn, err)
				return
			}

			// Re-get the same, make sure they are the same
			srcg2, err := sm.srcCoord.getSourceGatewayFor(ctx, lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error re-getting source: %s", nn, err)
			} else if srcg != srcg2 {
				t.Errorf("(src %q) first and second sources are not eq", nn)
			}

			// All of them _should_ select https, so this should work
			lpi.Source = "https://" + lpi.Source
			srcg3, err := sm.srcCoord.getSourceGatewayFor(ctx, lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error getting explicit https source: %s", nn, err)
			} else if srcg != srcg3 {
				t.Errorf("(src %q) explicit https source should reuse autodetected https source", nn)
			}

			// Now put in http, and they should differ
			lpi.Source = "http://" + string(lpi.ProjectRoot)
			srcg4, err := sm.srcCoord.getSourceGatewayFor(ctx, lpi)
			if err != nil {
				t.Errorf("(src %q) unexpected error getting explicit http source: %s", nn, err)
			} else if srcg == srcg4 {
				t.Errorf("(src %q) explicit http source should create a new src", nn)
			}
		}(pi)
	}

	wg.Wait()

	// nine entries (of which three are dupes): for each vcs, raw import path,
	// the https url, and the http url
	if len(sm.srcCoord.srcs) != 9 {
		t.Errorf("Should have nine discrete entries in the srcs map, got %v", len(sm.srcCoord.srcs))
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

	id := mkPI("github.com/sdboyer/gpkt").normalize()

	_, _, err := sm.GetManifestAndLock(id, NewVersion("v1.0.0"))
	if err != nil {
		t.Errorf("Unexpected error from GetInfoAt %s", err)
	}

	v, err := sm.ListVersions(id)
	if err != nil {
		t.Errorf("Unexpected error from ListVersions %s", err)
	}

	if len(v) != 7 {
		t.Errorf("Expected seven results from ListVersions, got %v", len(v))
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
	if sm.deduceCoord.rootxt.Len() != 1 {
		t.Errorf("Root path trie should have one element after one deduction, has %v", sm.deduceCoord.rootxt.Len())
	}

	pr, err = sm.DeduceProjectRoot(in)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", in, err)
	} else if string(pr) != in {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.deduceCoord.rootxt.Len() != 1 {
		t.Errorf("Root path trie should still have one element after performing the same deduction twice; has %v", sm.deduceCoord.rootxt.Len())
	}

	// Now do a subpath
	sub := path.Join(in, "foo")
	pr, err = sm.DeduceProjectRoot(sub)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", sub, err)
	} else if string(pr) != in {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.deduceCoord.rootxt.Len() != 2 {
		t.Errorf("Root path trie should have two elements, one for root and one for subpath; has %v", sm.deduceCoord.rootxt.Len())
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
	if sm.deduceCoord.rootxt.Len() != 4 {
		t.Errorf("Root path trie should have four elements, one for each unique root and subpath; has %v", sm.deduceCoord.rootxt.Len())
	}

	// Ensure that our prefixes are bounded by path separators
	in4 := "github.com/bagel/loxx"
	pr, err = sm.DeduceProjectRoot(in4)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", in4, err)
	} else if string(pr) != in4 {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.deduceCoord.rootxt.Len() != 5 {
		t.Errorf("Root path trie should have five elements, one for each unique root and subpath; has %v", sm.deduceCoord.rootxt.Len())
	}

	// Ensure that vcs extension-based matching comes through
	in5 := "ffffrrrraaaaaapppppdoesnotresolve.com/baz.git"
	pr, err = sm.DeduceProjectRoot(in5)
	if err != nil {
		t.Errorf("Problem while detecting root of %q %s", in5, err)
	} else if string(pr) != in5 {
		t.Errorf("Wrong project root was deduced;\n\t(GOT) %s\n\t(WNT) %s", pr, in)
	}
	if sm.deduceCoord.rootxt.Len() != 6 {
		t.Errorf("Root path trie should have six elements, one for each unique root and subpath; has %v", sm.deduceCoord.rootxt.Len())
	}
}

func TestMultiFetchThreadsafe(t *testing.T) {
	// This test is quite slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping slow test in short mode")
	}

	t.Skip("UGH: this is demonstrating real concurrency problems; skipping until we've fixed them")

	// FIXME test case of base path vs. e.g. https path - folding those together
	// is crucial
	projects := []ProjectIdentifier{
		mkPI("github.com/sdboyer/gps"),
		mkPI("github.com/sdboyer/gpkt"),
		mkPI("github.com/sdboyer/gogl"),
		mkPI("github.com/sdboyer/gliph"),
		mkPI("github.com/sdboyer/frozone"),
		mkPI("gopkg.in/sdboyer/gpkt.v1"),
		mkPI("gopkg.in/sdboyer/gpkt.v2"),
		mkPI("github.com/Masterminds/VCSTestRepo"),
		mkPI("github.com/go-yaml/yaml"),
		mkPI("github.com/Sirupsen/logrus"),
		mkPI("github.com/Masterminds/semver"),
		mkPI("github.com/Masterminds/vcs"),
		//mkPI("bitbucket.org/sdboyer/withbm"),
		//mkPI("bitbucket.org/sdboyer/nobm"),
	}

	// 40 gives us ten calls per op, per project, which should be(?) decently
	// likely to reveal underlying parallelism problems

	do := func(sm *SourceMgr) {
		wg := &sync.WaitGroup{}
		cnum := len(projects) * 40

		for i := 0; i < cnum; i++ {
			wg.Add(1)

			go func(id ProjectIdentifier, pass int) {
				switch pass {
				case 0:
					t.Logf("Deducing root for %s", id.errString())
					_, err := sm.DeduceProjectRoot(string(id.ProjectRoot))
					if err != nil {
						t.Errorf("err on deducing project root for %s: %s", id.errString(), err.Error())
					}
				case 1:
					t.Logf("syncing %s", id)
					err := sm.SyncSourceFor(id)
					if err != nil {
						t.Errorf("syncing failed for %s with err %s", id.errString(), err.Error())
					}
				case 2:
					t.Logf("listing versions for %s", id)
					_, err := sm.ListVersions(id)
					if err != nil {
						t.Errorf("listing versions failed for %s with err %s", id.errString(), err.Error())
					}
				case 3:
					t.Logf("Checking source existence for %s", id)
					y, err := sm.SourceExists(id)
					if err != nil {
						t.Errorf("err on checking source existence for %s: %s", id.errString(), err.Error())
					}
					if !y {
						t.Errorf("claims %s source does not exist", id.errString())
					}
				default:
					panic(fmt.Sprintf("wtf, %s %v", id, pass))
				}
				wg.Done()
			}(projects[i%len(projects)], (i/len(projects))%4)

			runtime.Gosched()
		}
		wg.Wait()
	}

	sm, _ := mkNaiveSM(t)
	do(sm)
	// Run the thing twice with a remade sm so that we cover both the cases of
	// pre-existing and new clones
	sm2, clean := remakeNaiveSM(sm, t)
	do(sm2)
	clean()
}

// Ensure that we don't see concurrent map writes when calling ListVersions.
// Regression test for https://github.com/sdboyer/gps/issues/156.
//
// Ideally this would be caught by TestMultiFetchThreadsafe, but perhaps the
// high degree of parallelism pretty much eliminates that as a realistic
// possibility?
func TestListVersionsRacey(t *testing.T) {
	// This test is quite slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping slow test in short mode")
	}

	sm, clean := mkNaiveSM(t)
	defer clean()

	wg := &sync.WaitGroup{}
	id := mkPI("github.com/sdboyer/gps")
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			_, err := sm.ListVersions(id)
			if err != nil {
				t.Errorf("listing versions failed with err %s", err.Error())
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

func TestErrAfterRelease(t *testing.T) {
	sm, clean := mkNaiveSM(t)
	clean()
	id := ProjectIdentifier{}

	_, err := sm.SourceExists(id)
	if err == nil {
		t.Errorf("SourceExists did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("SourceExists errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}

	err = sm.SyncSourceFor(id)
	if err == nil {
		t.Errorf("SyncSourceFor did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("SyncSourceFor errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}

	_, err = sm.ListVersions(id)
	if err == nil {
		t.Errorf("ListVersions did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("ListVersions errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}

	_, err = sm.RevisionPresentIn(id, "")
	if err == nil {
		t.Errorf("RevisionPresentIn did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("RevisionPresentIn errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}

	_, err = sm.ListPackages(id, nil)
	if err == nil {
		t.Errorf("ListPackages did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("ListPackages errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}

	_, _, err = sm.GetManifestAndLock(id, nil)
	if err == nil {
		t.Errorf("GetManifestAndLock did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("GetManifestAndLock errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}

	err = sm.ExportProject(id, nil, "")
	if err == nil {
		t.Errorf("ExportProject did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("ExportProject errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}

	_, err = sm.DeduceProjectRoot("")
	if err == nil {
		t.Errorf("DeduceProjectRoot did not error after calling Release()")
	} else if terr, ok := err.(smIsReleased); !ok {
		t.Errorf("DeduceProjectRoot errored after Release(), but with unexpected error: %T %s", terr, terr.Error())
	}
}

func TestSignalHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow test in short mode")
	}

	sm, clean := mkNaiveSM(t)
	//get self proc
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal("cannot find self proc")
	}

	sigch := make(chan os.Signal)
	sm.HandleSignals(sigch)

	sigch <- os.Interrupt
	<-time.After(10 * time.Millisecond)

	if atomic.LoadInt32(&sm.releasing) != 1 {
		t.Error("Releasing flag did not get set")
	}

	lpath := filepath.Join(sm.cachedir, "sm.lock")
	if _, err := os.Stat(lpath); err == nil {
		t.Fatal("Expected error on statting what should be an absent lock file")
	}
	clean()

	sm, clean = mkNaiveSM(t)
	sm.UseDefaultSignalHandling()
	go sm.DeduceProjectRoot("rsc.io/pdf")
	runtime.Gosched()

	// signal the process and call release right afterward
	now := time.Now()
	proc.Signal(os.Interrupt)
	sigdur := time.Since(now)
	t.Logf("time to send signal: %v", sigdur)
	sm.Release()
	reldur := time.Since(now) - sigdur
	t.Logf("time to return from Release(): %v", reldur)

	if reldur < 10*time.Millisecond {
		t.Errorf("finished too fast (%v); the necessary network request could not have completed yet", reldur)
	}
	if atomic.LoadInt32(&sm.releasing) != 1 {
		t.Error("Releasing flag did not get set")
	}

	lpath = filepath.Join(sm.cachedir, "sm.lock")
	if _, err := os.Stat(lpath); err == nil {
		t.Error("Expected error on statting what should be an absent lock file")
	}
	clean()

	sm, clean = mkNaiveSM(t)
	sm.UseDefaultSignalHandling()
	sm.StopSignalHandling()
	sm.UseDefaultSignalHandling()

	go sm.DeduceProjectRoot("rsc.io/pdf")
	//runtime.Gosched()
	// Ensure that it all works after teardown and re-set up
	proc.Signal(os.Interrupt)
	// Wait for twice the time it took to do it last time; should be safe
	<-time.After(reldur * 2)

	// proc.Signal doesn't send for windows, so just force it
	if runtime.GOOS == "windows" {
		sm.Release()
	}

	if atomic.LoadInt32(&sm.releasing) != 1 {
		t.Error("Releasing flag did not get set")
	}

	lpath = filepath.Join(sm.cachedir, "sm.lock")
	if _, err := os.Stat(lpath); err == nil {
		t.Fatal("Expected error on statting what should be an absent lock file")
	}
	clean()
}

func TestUnreachableSource(t *testing.T) {
	// If a git remote is unreachable (maybe the server is only accessible behind a VPN, or
	// something), we should return a clear error, not a panic.

	sm, clean := mkNaiveSM(t)
	defer clean()

	id := mkPI("golang.org/notareal/repo").normalize()
	_, err := sm.ListVersions(id)
	if err == nil {
		t.Error("expected err when listing versions of a bogus source, but got nil")
	}
}

func TestCallManager(t *testing.T) {
	bgc := context.Background()
	ctx, cancelFunc := context.WithCancel(bgc)
	cm := newCallManager(ctx)

	ci := callInfo{
		name: "foo",
		typ:  0,
	}

	_, err := cm.run(ci)
	if err != nil {
		t.Fatal("unexpected err on setUpCall:", err)
	}

	tc, exists := cm.running[ci]
	if !exists {
		t.Fatal("running call not recorded in map")
	}

	if tc.count != 1 {
		t.Fatalf("wrong count of running ci: wanted 1 got %v", tc.count)
	}

	// run another, but via setUpCall
	_, doneFunc, err := cm.setUpCall(bgc, "foo", 0)
	if err != nil {
		t.Fatal("unexpected err on setUpCall:", err)
	}

	tc, exists = cm.running[ci]
	if !exists {
		t.Fatal("running call not recorded in map")
	}

	if tc.count != 2 {
		t.Fatalf("wrong count of running ci: wanted 2 got %v", tc.count)
	}

	doneFunc()
	if len(cm.ran) != 0 {
		t.Fatal("should not record metrics until last one drops")
	}

	tc, exists = cm.running[ci]
	if !exists {
		t.Fatal("running call not recorded in map")
	}

	if tc.count != 1 {
		t.Fatalf("wrong count of running ci: wanted 1 got %v", tc.count)
	}

	cm.done(ci)
	ran, exists := cm.ran[0]
	if !exists {
		t.Fatal("should have metrics after closing last of a ci, but did not")
	}

	if ran.count != 1 {
		t.Fatalf("wrong count of serial runs of a call: wanted 1 got %v", ran.count)
	}

	cancelFunc()
	_, err = cm.run(ci)
	if err == nil {
		t.Fatal("should have errored on cm.run() after canceling cm's input context")
	}
}
