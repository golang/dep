package gps

import (
	"context"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/sdboyer/gps/pkgtree"
)

// Executed in parallel by TestSlowVcs
func testSourceGateway(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping gateway testing in short mode")
	}
	requiresBins(t, "git")

	cachedir, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Fatalf("failed to create temp dir: %s", err)
	}
	bgc := context.Background()
	ctx, cancelFunc := context.WithCancel(bgc)
	defer func() {
		removeAll(cachedir)
		cancelFunc()
	}()

	do := func(wantstate sourceState) func(t *testing.T) {
		return func(t *testing.T) {
			superv := newSupervisor(ctx)
			sc := newSourceCoordinator(superv, newDeductionCoordinator(superv), cachedir)

			id := mkPI("github.com/sdboyer/deptest")
			sg, err := sc.getSourceGatewayFor(ctx, id)
			if err != nil {
				t.Fatal(err)
			}

			if _, ok := sg.src.(*gitSource); !ok {
				t.Fatalf("Expected a gitSource, got a %T", sg.src)
			}

			if sg.srcState != wantstate {
				t.Fatalf("expected state on initial create to be %v, got %v", wantstate, sg.srcState)
			}

			if err := sg.syncLocal(ctx); err != nil {
				t.Fatalf("error on cloning git repo: %s", err)
			}

			cvlist := sg.cache.getAllVersions()
			if len(cvlist) != 4 {
				t.Fatalf("repo setup should've cached four versions, got %v: %s", len(cvlist), cvlist)
			}

			wanturl := "https://" + id.normalizedSource()
			goturl, err := sg.sourceURL(ctx)
			if err != nil {
				t.Fatalf("got err from sourceURL: %s", err)
			}
			if wanturl != goturl {
				t.Fatalf("Expected %s as source URL, got %s", wanturl, goturl)
			}

			vlist, err := sg.listVersions(ctx)
			if err != nil {
				t.Fatalf("Unexpected error getting version pairs from git repo: %s", err)
			}

			if len(vlist) != 4 {
				t.Fatalf("git test repo should've produced four versions, got %v: vlist was %s", len(vlist), vlist)
			} else {
				SortPairedForUpgrade(vlist)
				evl := []PairedVersion{
					NewVersion("v1.0.0").Is(Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")),
					NewVersion("v0.8.1").Is(Revision("3f4c3bea144e112a69bbe5d8d01c1b09a544253f")),
					NewVersion("v0.8.0").Is(Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")),
					newDefaultBranch("master").Is(Revision("3f4c3bea144e112a69bbe5d8d01c1b09a544253f")),
				}
				if !reflect.DeepEqual(vlist, evl) {
					t.Fatalf("Version list was not what we expected:\n\t(GOT): %s\n\t(WNT): %s", vlist, evl)
				}
			}

			rev := Revision("c575196502940c07bf89fd6d95e83b999162e051")
			// check that an expected rev is not in cache
			_, has := sg.cache.getVersionsFor(rev)
			if has {
				t.Fatal("shouldn't have bare revs in cache without specifically requesting them")
			}

			is, err := sg.revisionPresentIn(ctx, Revision("c575196502940c07bf89fd6d95e83b999162e051"))
			if err != nil {
				t.Fatalf("unexpected error while checking revision presence: %s", err)
			} else if !is {
				t.Fatalf("revision that should exist was not present")
			}

			// check that an expected rev is not in cache
			_, has = sg.cache.getVersionsFor(rev)
			if !has {
				t.Fatal("bare rev should be in cache after specific request for it")
			}

			// Ensure that a bad rev doesn't work on any method that takes
			// versions
			badver := NewVersion("notexist")
			wanterr := fmt.Errorf("version %q does not exist in source", badver)

			_, _, err = sg.getManifestAndLock(ctx, ProjectRoot("github.com/sdboyer/deptest"), badver, naiveAnalyzer{})
			if err == nil {
				t.Fatal("wanted err on nonexistent version")
			} else if err.Error() != wanterr.Error() {
				t.Fatalf("wanted nonexistent err when passing bad version, got: %s", err)
			}

			_, err = sg.listPackages(ctx, ProjectRoot("github.com/sdboyer/deptest"), badver)
			if err == nil {
				t.Fatal("wanted err on nonexistent version")
			} else if err.Error() != wanterr.Error() {
				t.Fatalf("wanted nonexistent err when passing bad version, got: %s", err)
			}

			err = sg.exportVersionTo(ctx, badver, cachedir)
			if err == nil {
				t.Fatal("wanted err on nonexistent version")
			} else if err.Error() != wanterr.Error() {
				t.Fatalf("wanted nonexistent err when passing bad version, got: %s", err)
			}

			wantptree := pkgtree.PackageTree{
				ImportRoot: "github.com/sdboyer/deptest",
				Packages: map[string]pkgtree.PackageOrErr{
					"github.com/sdboyer/deptest": pkgtree.PackageOrErr{
						P: pkgtree.Package{
							ImportPath: "github.com/sdboyer/deptest",
							Name:       "deptest",
							Imports:    []string{},
						},
					},
				},
			}

			ptree, err := sg.listPackages(ctx, ProjectRoot("github.com/sdboyer/deptest"), Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf"))
			if err != nil {
				t.Fatalf("unexpected err when getting package tree with known rev: %s", err)
			}
			if !reflect.DeepEqual(wantptree, ptree) {
				t.Fatalf("got incorrect PackageTree:\n\t(GOT): %#v\n\t(WNT): %#v", wantptree, ptree)
			}

			ptree, err = sg.listPackages(ctx, ProjectRoot("github.com/sdboyer/deptest"), NewVersion("v1.0.0"))
			if err != nil {
				t.Fatalf("unexpected err when getting package tree with unpaired good version: %s", err)
			}
			if !reflect.DeepEqual(wantptree, ptree) {
				t.Fatalf("got incorrect PackageTree:\n\t(GOT): %#v\n\t(WNT): %#v", wantptree, ptree)
			}
		}
	}

	// Run test twice so that we cover both the existing and non-existing case;
	// only difference in results is the initial setup state.
	t.Run("empty", do(sourceIsSetUp|sourceExistsUpstream|sourceHasLatestVersionList))
	t.Run("exists", do(sourceIsSetUp|sourceExistsLocally|sourceExistsUpstream|sourceHasLatestVersionList))
}
