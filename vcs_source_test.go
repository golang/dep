package gps

import (
	"context"
	"io/ioutil"
	"net/url"
	"os/exec"
	"reflect"
	"sync"
	"testing"
)

// Parent test that executes all the slow vcs interaction tests in parallel.
func TestSlowVcs(t *testing.T) {
	t.Run("write-deptree", testWriteDepTree)
	t.Run("source-gateway", testSourceGateway)
	t.Run("bzr-repo", testBzrRepo)
	t.Run("bzr-source", testBzrSourceInteractions)
	t.Run("svn-repo", testSvnRepo)
	// TODO(sdboyer) svn-source
	t.Run("hg-repo", testHgRepo)
	t.Run("hg-source", testHgSourceInteractions)
	t.Run("git-repo", testGitRepo)
	t.Run("git-source", testGitSourceInteractions)
	t.Run("gopkgin-source", testGopkginSourceInteractions)
}

func testGitSourceInteractions(t *testing.T) {
	t.Parallel()

	// This test is slowish, skip it on -short
	if testing.Short() {
		t.Skip("Skipping git source version fetching test in short mode")
	}
	requiresBins(t, "git")

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
	}
	defer func() {
		if err := removeAll(cpath); err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}()

	n := "github.com/sdboyer/gpkt"
	un := "https://" + n
	u, err := url.Parse(un)
	if err != nil {
		t.Fatalf("Error parsing URL %s: %s", un, err)
	}
	mb := maybeGitSource{
		url: u,
	}

	ctx := context.Background()
	superv := newSupervisor(ctx)
	isrc, state, err := mb.try(ctx, cpath, newMemoryCache(), superv)
	if err != nil {
		t.Fatalf("Unexpected error while setting up gitSource for test repo: %s", err)
	}

	wantstate := sourceIsSetUp | sourceExistsUpstream | sourceHasLatestVersionList
	if state != wantstate {
		t.Errorf("Expected return state to be %v, got %v", wantstate, state)
	}

	err = isrc.initLocal(ctx)
	if err != nil {
		t.Fatalf("Error on cloning git repo: %s", err)
	}

	src, ok := isrc.(*gitSource)
	if !ok {
		t.Fatalf("Expected a gitSource, got a %T", isrc)
	}

	if un != src.upstreamURL() {
		t.Errorf("Expected %s as source URL, got %s", un, src.upstreamURL())
	}

	pvlist, err := src.listVersions(ctx)
	if err != nil {
		t.Fatalf("Unexpected error getting version pairs from git repo: %s", err)
	}

	vlist := hidePair(pvlist)
	// check that an expected rev is present
	is, err := src.revisionPresentIn(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e"))
	if err != nil {
		t.Errorf("Unexpected error while checking revision presence: %s", err)
	} else if !is {
		t.Errorf("Revision that should exist was not present")
	}

	if len(vlist) != 7 {
		t.Errorf("git test repo should've produced seven versions, got %v: vlist was %s", len(vlist), vlist)
	} else {
		SortForUpgrade(vlist)
		evl := []Version{
			NewVersion("v2.0.0").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
			NewVersion("v1.1.0").Is(Revision("b2cb48dda625f6640b34d9ffb664533359ac8b91")),
			NewVersion("v1.0.0").Is(Revision("bf85021c0405edbc4f3648b0603818d641674f72")),
			newDefaultBranch("master").Is(Revision("bf85021c0405edbc4f3648b0603818d641674f72")),
			NewBranch("v1").Is(Revision("e3777f683305eafca223aefe56b4e8ecf103f467")),
			NewBranch("v1.1").Is(Revision("f1fbc520489a98306eb28c235204e39fa8a89c84")),
			NewBranch("v3").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
		}
		if !reflect.DeepEqual(vlist, evl) {
			t.Errorf("Version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
		}
	}

	// recheck that rev is present, this time interacting with cache differently
	is, err = src.revisionPresentIn(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e"))
	if err != nil {
		t.Errorf("Unexpected error while re-checking revision presence: %s", err)
	} else if !is {
		t.Errorf("Revision that should exist was not present on re-check")
	}
}

func testGopkginSourceInteractions(t *testing.T) {
	t.Parallel()

	// This test is slowish, skip it on -short
	if testing.Short() {
		t.Skip("Skipping gopkg.in source version fetching test in short mode")
	}
	requiresBins(t, "git")

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
	}
	defer func() {
		if err := removeAll(cpath); err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}()

	tfunc := func(opath, n string, major uint64, evl []Version) {
		un := "https://" + n
		u, err := url.Parse(un)
		if err != nil {
			t.Errorf("URL was bad, lolwut? errtext: %s", err)
			return
		}
		mb := maybeGopkginSource{
			opath: opath,
			url:   u,
			major: major,
		}

		ctx := context.Background()
		superv := newSupervisor(ctx)
		isrc, state, err := mb.try(ctx, cpath, newMemoryCache(), superv)
		if err != nil {
			t.Errorf("Unexpected error while setting up gopkginSource for test repo: %s", err)
			return
		}

		wantstate := sourceIsSetUp | sourceExistsUpstream | sourceHasLatestVersionList
		if state != wantstate {
			t.Errorf("Expected return state to be %v, got %v", wantstate, state)
		}

		err = isrc.initLocal(ctx)
		if err != nil {
			t.Fatalf("Error on cloning git repo: %s", err)
		}

		src, ok := isrc.(*gopkginSource)
		if !ok {
			t.Errorf("Expected a gopkginSource, got a %T", isrc)
			return
		}

		if un != src.upstreamURL() {
			t.Errorf("Expected %s as source URL, got %s", un, src.upstreamURL())
		}
		if src.major != major {
			t.Errorf("Expected %v as major version filter on gopkginSource, got %v", major, src.major)
		}

		// check that an expected rev is present
		rev := evl[0].(PairedVersion).Underlying()
		is, err := src.revisionPresentIn(rev)
		if err != nil {
			t.Errorf("Unexpected error while checking revision presence: %s", err)
		} else if !is {
			t.Errorf("Revision %s that should exist was not present", rev)
		}

		pvlist, err := src.listVersions(ctx)
		if err != nil {
			t.Errorf("Unexpected error getting version pairs from hg repo: %s", err)
		}

		vlist := hidePair(pvlist)
		if len(vlist) != len(evl) {
			t.Errorf("gopkgin test repo should've produced %v versions, got %v", len(evl), len(vlist))
		} else {
			SortForUpgrade(vlist)
			if !reflect.DeepEqual(vlist, evl) {
				t.Errorf("Version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
			}
		}

		// Run again, this time to ensure cache outputs correctly
		pvlist, err = src.listVersions(ctx)
		if err != nil {
			t.Errorf("Unexpected error getting version pairs from hg repo: %s", err)
		}

		vlist = hidePair(pvlist)
		if len(vlist) != len(evl) {
			t.Errorf("gopkgin test repo should've produced %v versions, got %v", len(evl), len(vlist))
		} else {
			SortForUpgrade(vlist)
			if !reflect.DeepEqual(vlist, evl) {
				t.Errorf("Version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
			}
		}

		// recheck that rev is present, this time interacting with cache differently
		is, err = src.revisionPresentIn(rev)
		if err != nil {
			t.Errorf("Unexpected error while re-checking revision presence: %s", err)
		} else if !is {
			t.Errorf("Revision that should exist was not present on re-check")
		}
	}

	// simultaneously run for v1, v2, and v3 filters of the target repo
	wg := &sync.WaitGroup{}
	wg.Add(3)
	go func() {
		tfunc("gopkg.in/sdboyer/gpkt.v1", "github.com/sdboyer/gpkt", 1, []Version{
			NewVersion("v1.1.0").Is(Revision("b2cb48dda625f6640b34d9ffb664533359ac8b91")),
			NewVersion("v1.0.0").Is(Revision("bf85021c0405edbc4f3648b0603818d641674f72")),
			newDefaultBranch("v1.1").Is(Revision("f1fbc520489a98306eb28c235204e39fa8a89c84")),
			NewBranch("v1").Is(Revision("e3777f683305eafca223aefe56b4e8ecf103f467")),
		})
		wg.Done()
	}()

	go func() {
		tfunc("gopkg.in/sdboyer/gpkt.v2", "github.com/sdboyer/gpkt", 2, []Version{
			NewVersion("v2.0.0").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
		})
		wg.Done()
	}()

	go func() {
		tfunc("gopkg.in/sdboyer/gpkt.v3", "github.com/sdboyer/gpkt", 3, []Version{
			newDefaultBranch("v3").Is(Revision("4a54adf81c75375d26d376459c00d5ff9b703e5e")),
		})
		wg.Done()
	}()

	wg.Wait()
}

func testBzrSourceInteractions(t *testing.T) {
	t.Parallel()

	// This test is quite slow (ugh bzr), so skip it on -short
	if testing.Short() {
		t.Skip("Skipping bzr source version fetching test in short mode")
	}
	requiresBins(t, "bzr")

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
	}
	defer func() {
		if err := removeAll(cpath); err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}()

	n := "launchpad.net/govcstestbzrrepo"
	un := "https://" + n
	u, err := url.Parse(un)
	if err != nil {
		t.Fatalf("Error parsing URL %s: %s", un, err)
	}
	mb := maybeBzrSource{
		url: u,
	}

	ctx := context.Background()
	superv := newSupervisor(ctx)
	isrc, state, err := mb.try(ctx, cpath, newMemoryCache(), superv)
	if err != nil {
		t.Fatalf("Unexpected error while setting up bzrSource for test repo: %s", err)
	}

	wantstate := sourceIsSetUp | sourceExistsUpstream
	if state != wantstate {
		t.Errorf("Expected return state to be %v, got %v", wantstate, state)
	}

	err = isrc.initLocal(ctx)
	if err != nil {
		t.Fatalf("Error on cloning git repo: %s", err)
	}

	src, ok := isrc.(*bzrSource)
	if !ok {
		t.Fatalf("Expected a bzrSource, got a %T", isrc)
	}

	if state != wantstate {
		t.Errorf("Expected return state to be %v, got %v", wantstate, state)
	}
	if un != src.upstreamURL() {
		t.Errorf("Expected %s as source URL, got %s", un, src.upstreamURL())
	}
	evl := []Version{
		NewVersion("1.0.0").Is(Revision("matt@mattfarina.com-20150731135137-pbphasfppmygpl68")),
		newDefaultBranch("(default)").Is(Revision("matt@mattfarina.com-20150731135137-pbphasfppmygpl68")),
	}

	// check that an expected rev is present
	is, err := src.revisionPresentIn(Revision("matt@mattfarina.com-20150731135137-pbphasfppmygpl68"))
	if err != nil {
		t.Errorf("Unexpected error while checking revision presence: %s", err)
	} else if !is {
		t.Errorf("Revision that should exist was not present")
	}

	pvlist, err := src.listVersions(ctx)
	if err != nil {
		t.Errorf("Unexpected error getting version pairs from bzr repo: %s", err)
	}

	vlist := hidePair(pvlist)
	if len(vlist) != 2 {
		t.Errorf("bzr test repo should've produced two versions, got %v", len(vlist))
	} else {
		SortForUpgrade(vlist)
		if !reflect.DeepEqual(vlist, evl) {
			t.Errorf("bzr version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
		}
	}

	// Run again, this time to ensure cache outputs correctly
	pvlist, err = src.listVersions(ctx)
	if err != nil {
		t.Errorf("Unexpected error getting version pairs from bzr repo: %s", err)
	}

	vlist = hidePair(pvlist)
	if len(vlist) != 2 {
		t.Errorf("bzr test repo should've produced two versions, got %v", len(vlist))
	} else {
		SortForUpgrade(vlist)
		if !reflect.DeepEqual(vlist, evl) {
			t.Errorf("bzr version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
		}
	}

	// recheck that rev is present, this time interacting with cache differently
	is, err = src.revisionPresentIn(Revision("matt@mattfarina.com-20150731135137-pbphasfppmygpl68"))
	if err != nil {
		t.Errorf("Unexpected error while re-checking revision presence: %s", err)
	} else if !is {
		t.Errorf("Revision that should exist was not present on re-check")
	}
}

func testHgSourceInteractions(t *testing.T) {
	t.Parallel()

	// This test is slow, so skip it on -short
	if testing.Short() {
		t.Skip("Skipping hg source version fetching test in short mode")
	}
	requiresBins(t, "hg")

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
	}
	defer func() {
		if err := removeAll(cpath); err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}()

	tfunc := func(n string, evl []Version) {
		un := "https://" + n
		u, err := url.Parse(un)
		if err != nil {
			t.Errorf("URL was bad, lolwut? errtext: %s", err)
			return
		}
		mb := maybeHgSource{
			url: u,
		}

		ctx := context.Background()
		superv := newSupervisor(ctx)
		isrc, state, err := mb.try(ctx, cpath, newMemoryCache(), superv)
		if err != nil {
			t.Errorf("Unexpected error while setting up hgSource for test repo: %s", err)
			return
		}

		wantstate := sourceIsSetUp | sourceExistsUpstream
		if state != wantstate {
			t.Errorf("Expected return state to be %v, got %v", wantstate, state)
		}

		err = isrc.initLocal(ctx)
		if err != nil {
			t.Fatalf("Error on cloning git repo: %s", err)
		}

		src, ok := isrc.(*hgSource)
		if !ok {
			t.Errorf("Expected a hgSource, got a %T", isrc)
			return
		}

		if state != wantstate {
			t.Errorf("Expected return state to be %v, got %v", wantstate, state)
		}
		if un != src.upstreamURL() {
			t.Errorf("Expected %s as source URL, got %s", un, src.upstreamURL())
		}

		// check that an expected rev is present
		is, err := src.revisionPresentIn(Revision("103d1bddef2199c80aad7c42041223083d613ef9"))
		if err != nil {
			t.Errorf("Unexpected error while checking revision presence: %s", err)
		} else if !is {
			t.Errorf("Revision that should exist was not present")
		}

		pvlist, err := src.listVersions(ctx)
		if err != nil {
			t.Errorf("Unexpected error getting version pairs from hg repo: %s", err)
		}

		vlist := hidePair(pvlist)
		if len(vlist) != len(evl) {
			t.Errorf("hg test repo should've produced %v versions, got %v", len(evl), len(vlist))
		} else {
			SortForUpgrade(vlist)
			if !reflect.DeepEqual(vlist, evl) {
				t.Errorf("Version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
			}
		}

		// Run again, this time to ensure cache outputs correctly
		pvlist, err = src.listVersions(ctx)
		if err != nil {
			t.Errorf("Unexpected error getting version pairs from hg repo: %s", err)
		}

		vlist = hidePair(pvlist)
		if len(vlist) != len(evl) {
			t.Errorf("hg test repo should've produced %v versions, got %v", len(evl), len(vlist))
		} else {
			SortForUpgrade(vlist)
			if !reflect.DeepEqual(vlist, evl) {
				t.Errorf("Version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
			}
		}

		// recheck that rev is present, this time interacting with cache differently
		is, err = src.revisionPresentIn(Revision("103d1bddef2199c80aad7c42041223083d613ef9"))
		if err != nil {
			t.Errorf("Unexpected error while re-checking revision presence: %s", err)
		} else if !is {
			t.Errorf("Revision that should exist was not present on re-check")
		}
	}

	// simultaneously run for both the repo with and without the magic bookmark
	donech := make(chan struct{})
	go func() {
		tfunc("bitbucket.org/sdboyer/withbm", []Version{
			NewVersion("v1.0.0").Is(Revision("aa110802a0c64195d0a6c375c9f66668827c90b4")),
			newDefaultBranch("@").Is(Revision("b10d05d581e5401f383e48ccfeb84b48fde99d06")),
			NewBranch("another").Is(Revision("b10d05d581e5401f383e48ccfeb84b48fde99d06")),
			NewBranch("default").Is(Revision("3d466f437f6616da594bbab6446cc1cb4328d1bb")),
			NewBranch("newbranch").Is(Revision("5e2a01be9aee942098e44590ae545c7143da9675")),
		})
		close(donech)
	}()

	tfunc("bitbucket.org/sdboyer/nobm", []Version{
		NewVersion("v1.0.0").Is(Revision("aa110802a0c64195d0a6c375c9f66668827c90b4")),
		newDefaultBranch("default").Is(Revision("3d466f437f6616da594bbab6446cc1cb4328d1bb")),
		NewBranch("another").Is(Revision("b10d05d581e5401f383e48ccfeb84b48fde99d06")),
		NewBranch("newbranch").Is(Revision("5e2a01be9aee942098e44590ae545c7143da9675")),
	})

	<-donech
}

// Fail a test if the specified binaries aren't installed.
func requiresBins(t *testing.T, bins ...string) {
	for _, b := range bins {
		_, err := exec.LookPath(b)
		if err != nil {
			t.Fatalf("%s is not installed", b)
		}
	}
}
