package gps

import (
	"io/ioutil"
	"net/url"
	"reflect"
	"sort"
	"testing"
)

func TestGitVersionFetching(t *testing.T) {
	// This test is quite slow, skip it on -short
	if testing.Short() {
		t.Skip("Skipping git source version fetching test in short mode")
	}

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
	}
	rf := func() {
		err := removeAll(cpath)
		if err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}

	n := "github.com/Masterminds/VCSTestRepo"
	u, err := url.Parse("https://" + n)
	if err != nil {
		t.Errorf("URL was bad, lolwut? errtext: %s", err)
		rf()
		t.FailNow()
	}
	mb := maybeGitSource{
		n:   n,
		url: u,
	}

	isrc, err := mb.try(cpath, naiveAnalyzer{})
	if err != nil {
		t.Errorf("Unexpected error while setting up gitSource for test repo: %s", err)
		rf()
		t.FailNow()
	}
	src, ok := isrc.(*gitSource)
	if !ok {
		t.Errorf("Expected a gitSource, got a %T", isrc)
		rf()
		t.FailNow()
	}

	vlist, err := src.listVersions()
	if err != nil {
		t.Errorf("Unexpected error getting version pairs from git repo: %s", err)
		rf()
		t.FailNow()
	}

	if src.ex.s&existsUpstream != existsUpstream {
		t.Errorf("gitSource.listVersions() should have set the upstream existence bit for search")
	}
	if src.ex.f&existsUpstream != existsUpstream {
		t.Errorf("gitSource.listVersions() should have set the upstream existence bit for found")
	}
	if src.ex.s&existsInCache != 0 {
		t.Errorf("gitSource.listVersions() should not have set the cache existence bit for search")
	}
	if src.ex.f&existsInCache != 0 {
		t.Errorf("gitSource.listVersions() should not have set the cache existence bit for found")
	}

	if len(vlist) != 3 {
		t.Errorf("git test repo should've produced three versions, got %v: vlist was %s", len(vlist), vlist)
	} else {
		sort.Sort(upgradeVersionSorter(vlist))
		evl := []Version{
			NewVersion("1.0.0").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")),
			NewBranch("master").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")),
			NewBranch("test").Is(Revision("30605f6ac35fcb075ad0bfa9296f90a7d891523e")),
		}
		if !reflect.DeepEqual(vlist, evl) {
			t.Errorf("Version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
		}
	}
}
