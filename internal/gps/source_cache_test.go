// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"io/ioutil"
	"log"
	"sort"
	"testing"
	"time"

	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

func Test_singleSourceCache(t *testing.T) {
	newMem := func(*testing.T, string, string) singleSourceCache { return newMemoryCache() }
	t.Run("mem", singleSourceCacheTest{newCache: newMem}.run)

	epoch := time.Now().Unix()
	newBolt := func(t *testing.T, cachedir, root string) singleSourceCache {
		pi := mkPI(root).normalize()
		c, err := newBoltCache(cachedir, pi, epoch, log.New(test.Writer{t}, "", 0))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	t.Run("bolt/open", singleSourceCacheTest{newCache: newBolt}.run)
	t.Run("bolt/refresh", singleSourceCacheTest{newCache: newBolt, persistent: true}.run)
}

var testAnalyzerInfo = ProjectAnalyzerInfo{
	Name:    "test-analyzer",
	Version: 1,
}

type singleSourceCacheTest struct {
	newCache   func(*testing.T, string, string) singleSourceCache
	persistent bool
}

// run tests singleSourceCache methods of caches returned by test.newCache.
// For test.persistent caches, test.newCache is periodically called mid-test to ensure persistence.
func (test singleSourceCacheTest) run(t *testing.T) {
	const root = "example.com/test"
	cpath, err := ioutil.TempDir("", "singlesourcecache")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %s", err)
	}

	t.Run("info", func(t *testing.T) {
		const rev Revision = "revision"

		c := test.newCache(t, cpath, root)
		defer func() {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
		}()

		var m Manifest = &cachedManifest{
			constraints: ProjectConstraints{
				ProjectRoot("foo"): ProjectProperties{
					Constraint: Any(),
				},
				ProjectRoot("bar"): ProjectProperties{
					Source:     "whatever",
					Constraint: testSemverConstraint(t, "> 1.3"),
				},
			},
			overrides: ProjectConstraints{
				ProjectRoot("b"): ProjectProperties{
					Constraint: testSemverConstraint(t, "2.0.0"),
				},
			},
			ignored: map[string]bool{
				"a": true,
				"b": true,
			},
			required: map[string]bool{
				"c": true,
				"d": true,
			},
		}
		var l Lock = &cachedLock{
			inputHash: []byte("test_hash"),
			projects: []LockedProject{
				NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), []string{"gps"}),
				NewLockedProject(mkPI("github.com/sdboyer/gps2"), NewVersion("v0.10.0"), nil),
				NewLockedProject(mkPI("github.com/sdboyer/gps3"), NewVersion("v0.10.0"), []string{"gps", "flugle"}),
				NewLockedProject(mkPI("foo"), NewVersion("nada"), []string{"foo"}),
				NewLockedProject(mkPI("github.com/sdboyer/gps4"), NewVersion("v0.10.0"), []string{"flugle", "gps"}),
			},
		}
		c.setManifestAndLock(rev, testAnalyzerInfo, m, l)

		if test.persistent {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
			c = test.newCache(t, cpath, root)
		}

		gotM, gotL, ok := c.getManifestAndLock(rev, testAnalyzerInfo)
		if !ok {
			t.Error("no manifest and lock found for revision")
		}
		compareManifests(t, m, gotM)
		if dl := DiffLocks(l, gotL); dl != nil {
			t.Errorf("lock differences:\n\t %#v", dl)
		}

		m = &cachedManifest{
			constraints: ProjectConstraints{
				ProjectRoot("foo"): ProjectProperties{
					Source:     "whatever",
					Constraint: Any(),
				},
			},
			overrides: ProjectConstraints{
				ProjectRoot("bar"): ProjectProperties{
					Constraint: testSemverConstraint(t, "2.0.0"),
				},
			},
			ignored: map[string]bool{
				"c": true,
				"d": true,
			},
			required: map[string]bool{
				"a": true,
				"b": true,
			},
		}
		l = &cachedLock{
			inputHash: []byte("different_test_hash"),
			projects: []LockedProject{
				NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0").Pair("278a227dfc3d595a33a77ff3f841fd8ca1bc8cd0"), []string{"gps"}),
				NewLockedProject(mkPI("github.com/sdboyer/gps2"), NewVersion("v0.11.0"), []string{"gps"}),
				NewLockedProject(mkPI("github.com/sdboyer/gps3"), Revision("278a227dfc3d595a33a77ff3f841fd8ca1bc8cd0"), []string{"gps"}),
			},
		}
		c.setManifestAndLock(rev, testAnalyzerInfo, m, l)

		if test.persistent {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
			c = test.newCache(t, cpath, root)
		}

		gotM, gotL, ok = c.getManifestAndLock(rev, testAnalyzerInfo)
		if !ok {
			t.Error("no manifest and lock found for revision")
		}
		compareManifests(t, m, gotM)
		if dl := DiffLocks(l, gotL); dl != nil {
			t.Errorf("lock differences:\n\t %#v", dl)
		}
	})

	t.Run("pkgTree", func(t *testing.T) {
		c := test.newCache(t, cpath, root)
		defer func() {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
		}()

		const rev Revision = "rev_adsfjkl"

		if got, ok := c.getPackageTree(rev); ok {
			t.Fatalf("unexpected result before setting package tree: %v", got)
		}

		if test.persistent {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
			c = test.newCache(t, cpath, root)
		}

		pt := pkgtree.PackageTree{
			ImportRoot: root,
			Packages: map[string]pkgtree.PackageOrErr{
				"simple": {
					P: pkgtree.Package{
						ImportPath:  "simple",
						CommentPath: "comment",
						Name:        "simple",
						Imports: []string{
							"github.com/golang/dep/internal/gps",
							"sort",
						},
					},
				},
				"m1p": {
					P: pkgtree.Package{
						ImportPath:  "m1p",
						CommentPath: "",
						Name:        "m1p",
						Imports: []string{
							"github.com/golang/dep/internal/gps",
							"os",
							"sort",
						},
					},
				},
			},
		}
		c.setPackageTree(rev, pt)

		if test.persistent {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
			c = test.newCache(t, cpath, root)
		}

		got, ok := c.getPackageTree(rev)
		if !ok {
			t.Errorf("no package tree found:\n\t(WNT): %#v", pt)
		}
		comparePackageTree(t, pt, got)

		if test.persistent {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
			c = test.newCache(t, cpath, root)
		}

		pt = pkgtree.PackageTree{
			ImportRoot: root,
			Packages: map[string]pkgtree.PackageOrErr{
				"test": {
					Err: errors.New("error"),
				},
			},
		}
		c.setPackageTree(rev, pt)

		if test.persistent {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
			c = test.newCache(t, cpath, root)
		}

		got, ok = c.getPackageTree(rev)
		if !ok {
			t.Errorf("no package tree found:\n\t(WNT): %#v", pt)
		}
		comparePackageTree(t, pt, got)
	})

	t.Run("versions", func(t *testing.T) {
		c := test.newCache(t, cpath, root)
		defer func() {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
		}()

		const rev1, rev2 = "rev1", "rev2"
		const br, ver = "branch_name", "2.10"
		versions := []PairedVersion{
			NewBranch(br).Pair(rev1),
			NewVersion(ver).Pair(rev2),
		}
		SortPairedForDowngrade(versions)
		c.setVersionMap(versions)

		if test.persistent {
			if err := c.close(); err != nil {
				t.Fatal("failed to close cache:", err)
			}
			c = test.newCache(t, cpath, root)
		}

		t.Run("getAllVersions", func(t *testing.T) {
			got := c.getAllVersions()
			if len(got) != len(versions) {
				t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", got, versions)
			} else {
				SortPairedForDowngrade(got)
				for i := range versions {
					if !versions[i].identical(got[i]) {
						t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", got, versions)
						break
					}
				}
			}
		})

		revToUV := map[Revision]UnpairedVersion{
			rev1: NewBranch(br),
			rev2: NewVersion(ver),
		}

		t.Run("getVersionsFor", func(t *testing.T) {
			for rev, want := range revToUV {
				rev, want := rev, want
				t.Run(string(rev), func(t *testing.T) {
					uvs, ok := c.getVersionsFor(rev)
					if !ok {
						t.Errorf("no version found:\n\t(WNT) %#v", want)
					} else if len(uvs) != 1 {
						t.Errorf("expected one result but got %d", len(uvs))
					} else {
						uv := uvs[0]
						if uv.Type() != want.Type() {
							t.Errorf("expected version type %d but got %d", want.Type(), uv.Type())
						}
						if uv.String() != want.String() {
							t.Errorf("expected version %q but got %q", want.String(), uv.String())
						}
					}
				})
			}
		})

		t.Run("getRevisionFor", func(t *testing.T) {
			for want, uv := range revToUV {
				want, uv := want, uv
				t.Run(uv.String(), func(t *testing.T) {
					rev, ok := c.getRevisionFor(uv)
					if !ok {
						t.Errorf("expected revision %q but got none", want)
					} else if rev != want {
						t.Errorf("expected revision %q but got %q", want, rev)
					}
				})
			}
		})

		t.Run("toRevision", func(t *testing.T) {
			for want, uv := range revToUV {
				want, uv := want, uv
				t.Run(uv.String(), func(t *testing.T) {
					rev, ok := c.toRevision(uv)
					if !ok {
						t.Errorf("expected revision %q but got none", want)
					} else if rev != want {
						t.Errorf("expected revision %q but got %q", want, rev)
					}
				})
			}
		})

		t.Run("toUnpaired", func(t *testing.T) {
			for rev, want := range revToUV {
				rev, want := rev, want
				t.Run(want.String(), func(t *testing.T) {
					uv, ok := c.toUnpaired(rev)
					if !ok {
						t.Errorf("no UnpairedVersion found:\n\t(WNT): %#v", uv)
					} else if !uv.identical(want) {
						t.Errorf("unexpected UnpairedVersion:\n\t(GOT): %#v\n\t(WNT): %#v", uv, want)
					}
				})
			}
		})
	})
}

// compareManifests compares two manifests and reports differences as test errors.
func compareManifests(t *testing.T, want, got Manifest) {
	if (want == nil || got == nil) && (got != nil || want != nil) {
		t.Errorf("one manifest is nil:\n\t(GOT): %#v\n\t(WNT): %#v", got, want)
		return
	}
	{
		want, got := want.DependencyConstraints(), got.DependencyConstraints()
		if !projectConstraintsEqual(want, got) {
			t.Errorf("unexpected constraints:\n\t(GOT): %#v\n\t(WNT): %#v", got, want)
		}
	}

	wantRM, wantOK := want.(RootManifest)
	gotRM, gotOK := got.(RootManifest)
	if wantOK && !gotOK {
		t.Errorf("expected RootManifest:\n\t(GOT): %#v", got)
		return
	}
	if gotOK && !wantOK {
		t.Errorf("didn't expected RootManifest:\n\t(GOT): %#v", got)
		return
	}

	{
		want, got := wantRM.IgnoredPackages(), gotRM.IgnoredPackages()
		if !mapStringBoolEqual(want, got) {
			t.Errorf("unexpected ignored packages:\n\t(GOT): %#v\n\t(WNT): %#v", got, want)
		}
	}

	{
		want, got := wantRM.Overrides(), gotRM.Overrides()
		if !projectConstraintsEqual(want, got) {
			t.Errorf("unexpected overrides:\n\t(GOT): %#v\n\t(WNT): %#v", got, want)
		}
	}

	{
		want, got := wantRM.RequiredPackages(), gotRM.RequiredPackages()
		if !mapStringBoolEqual(want, got) {
			t.Errorf("unexpected required packages:\n\t(GOT): %#v\n\t(WNT): %#v", got, want)
		}
	}
}

// comparePackageTree compares two pkgtree.PackageTree and reports differences as test errors.
func comparePackageTree(t *testing.T, want, got pkgtree.PackageTree) {
	if got.ImportRoot != want.ImportRoot {
		t.Errorf("expected package tree root %q but got %q", want.ImportRoot, got.ImportRoot)
	}
	{
		want, got := want.Packages, got.Packages
		if len(want) != len(got) {
			t.Errorf("unexpected packages:\n\t(GOT): %#v\n\t(WNT): %#v", got, want)
		} else {
			for k, v := range want {
				if v2, ok := got[k]; !ok {
					t.Errorf("key %q: expected %v but got none", k, v)
				} else if !packageOrErrEqual(v, v2) {
					t.Errorf("key %q: expected %v but got %v", k, v, v2)
				}
			}
		}
	}
}

func projectConstraintsEqual(want, got ProjectConstraints) bool {
	loop, check := want, got
	if len(got) > len(want) {
		loop, check = got, want
	}
	for pr, pp := range loop {
		pp2, ok := check[pr]
		if !ok {
			return false
		}
		if pp.Source != pp2.Source {
			return false
		}
		if pp.Constraint == nil || pp2.Constraint == nil {
			if pp.Constraint != nil || pp2.Constraint != nil {
				return false
			}
		} else if !pp.Constraint.identical(pp2.Constraint) {
			return false
		}
	}
	return true
}

func mapStringBoolEqual(exp, got map[string]bool) bool {
	loop, check := exp, got
	if len(got) > len(exp) {
		loop, check = got, exp
	}
	for k, v := range loop {
		v2, ok := check[k]
		if !ok || v != v2 {
			return false
		}
	}
	return true
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// packageOrErrEqual return true if the pkgtree.PackageOrErrs are equal. Error equality is
// string based. Imports and TestImports are treated as sets, and will be sorted.
func packageOrErrEqual(a, b pkgtree.PackageOrErr) bool {
	if safeError(a.Err) != safeError(b.Err) {
		return false
	}
	if a.P.Name != b.P.Name {
		return false
	}
	if a.P.ImportPath != b.P.ImportPath {
		return false
	}
	if a.P.CommentPath != b.P.CommentPath {
		return false
	}

	if len(a.P.Imports) != len(b.P.Imports) {
		return false
	}
	sort.Strings(a.P.Imports)
	sort.Strings(b.P.Imports)
	for i := range a.P.Imports {
		if a.P.Imports[i] != b.P.Imports[i] {
			return false
		}
	}

	if len(a.P.TestImports) != len(b.P.TestImports) {
		return false
	}
	sort.Strings(a.P.TestImports)
	sort.Strings(b.P.TestImports)
	for i := range a.P.TestImports {
		if a.P.TestImports[i] != b.P.TestImports[i] {
			return false
		}
	}

	return true
}
