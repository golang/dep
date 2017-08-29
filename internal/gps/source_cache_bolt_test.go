// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/golang/dep/internal/test"
)

func TestBoltCacheTimeout(t *testing.T) {
	const root = "example.com/test"
	cpath, err := ioutil.TempDir("", "singlesourcecache")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %s", err)
	}
	pi := ProjectIdentifier{ProjectRoot: root}
	logger := log.New(test.Writer{t}, "", 0)

	start := time.Now()
	c, err := newBoltCache(cpath, pi, start.Unix(), logger)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()

	rev := Revision("test")
	ai := ProjectAnalyzerInfo{Name: "name", Version: 42}

	manifest := &cachedManifest{
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
	}

	lock := &cachedLock{
		inputHash: []byte("test_hash"),
		projects: []LockedProject{
			NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), []string{"gps"}),
			NewLockedProject(mkPI("github.com/sdboyer/gps2"), NewVersion("v0.10.0"), nil),
			NewLockedProject(mkPI("github.com/sdboyer/gps3"), NewVersion("v0.10.0"), []string{"gps", "flugle"}),
			NewLockedProject(mkPI("foo"), NewVersion("nada"), []string{"foo"}),
			NewLockedProject(mkPI("github.com/sdboyer/gps4"), NewVersion("v0.10.0"), []string{"flugle", "gps"}),
		},
	}

	ptree := pkgtree.PackageTree{
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

	pvs := []PairedVersion{
		NewBranch("originalbranch").Pair("rev1"),
		NewVersion("originalver").Pair("rev2"),
	}

	// Write values timestamped > start.
	{
		c.setManifestAndLock(rev, ai, manifest, lock)
		c.setPackageTree(rev, ptree)
		c.setVersionMap(pvs)
	}
	// Read back values timestamped > start.
	{
		gotM, gotL, ok := c.getManifestAndLock(rev, ai)
		if !ok {
			t.Error("no manifest and lock found for revision")
		}
		compareManifests(t, manifest, gotM)
		if dl := DiffLocks(lock, gotL); dl != nil {
			t.Errorf("lock differences:\n\t %#v", dl)
		}

		got, ok := c.getPackageTree(rev)
		if !ok {
			t.Errorf("no package tree found:\n\t(WNT): %#v", ptree)
		}
		comparePackageTree(t, ptree, got)

		gotV := c.getAllVersions()
		if len(gotV) != len(pvs) {
			t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", gotV, pvs)
		} else {
			SortPairedForDowngrade(gotV)
			for i := range pvs {
				if !pvs[i].identical(gotV[i]) {
					t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", gotV, pvs)
					break
				}
			}
		}
	}

	if err := c.close(); err != nil {
		t.Fatal("failed to close cache:", err)
	}

	// Read with a later epoch. Expect no values, since all timestamped < after.
	{
		after := time.Now()
		if after.Unix() <= start.Unix() {
			// Ensure a future timestamp.
			after = start.Add(10 * time.Second)
		}
		c, err = newBoltCache(cpath, pi, after.Unix(), logger)
		if err != nil {
			t.Fatal(err)
		}

		m, l, ok := c.getManifestAndLock(rev, ai)
		if ok {
			t.Errorf("expected no cached info, but got:\n\tManifest: %#v\n\tLock: %#v\n", m, l)
		}

		ptree, ok := c.getPackageTree(rev)
		if ok {
			t.Errorf("expected no cached package tree, but got:\n\t%#v", ptree)
		}

		pvs := c.getAllVersions()
		if len(pvs) > 0 {
			t.Errorf("expected no cached versions, but got:\n\t%#v", pvs)
		}
	}

	if err := c.close(); err != nil {
		t.Fatal("failed to close cache:", err)
	}

	// Re-connect with the original epoch.
	c, err = newBoltCache(cpath, pi, start.Unix(), logger)
	if err != nil {
		t.Fatal(err)
	}
	// Read values timestamped > start.
	{
		gotM, gotL, ok := c.getManifestAndLock(rev, ai)
		if !ok {
			t.Error("no manifest and lock found for revision")
		}
		compareManifests(t, manifest, gotM)
		if dl := DiffLocks(lock, gotL); dl != nil {
			t.Errorf("lock differences:\n\t %#v", dl)
		}

		got, ok := c.getPackageTree(rev)
		if !ok {
			t.Errorf("no package tree found:\n\t(WNT): %#v", ptree)
		}
		comparePackageTree(t, ptree, got)

		gotV := c.getAllVersions()
		if len(gotV) != len(pvs) {
			t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", gotV, pvs)
		} else {
			SortPairedForDowngrade(gotV)
			for i := range pvs {
				if !pvs[i].identical(gotV[i]) {
					t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", gotV, pvs)
					break
				}
			}
		}
	}

	// New values.
	newManifest := &cachedManifest{
		constraints: ProjectConstraints{
			ProjectRoot("foo"): ProjectProperties{
				Constraint: NewBranch("master"),
			},
			ProjectRoot("bar"): ProjectProperties{
				Source:     "whatever",
				Constraint: testSemverConstraint(t, "> 1.5"),
			},
		},
	}

	newLock := &cachedLock{
		inputHash: []byte("new_test_hash"),
		projects: []LockedProject{
			NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v1"), []string{"gps"}),
		},
	}

	newPtree := pkgtree.PackageTree{
		ImportRoot: root,
		Packages: map[string]pkgtree.PackageOrErr{
			"simple": {
				P: pkgtree.Package{
					ImportPath:  "simple",
					CommentPath: "newcomment",
					Name:        "simple",
					Imports: []string{
						"github.com/golang/dep/internal/gps42",
						"test",
					},
				},
			},
			"m1p": {
				P: pkgtree.Package{
					ImportPath:  "m1p",
					CommentPath: "",
					Name:        "m1p",
					Imports: []string{
						"os",
					},
				},
			},
		},
	}

	newPVS := []PairedVersion{
		NewBranch("newbranch").Pair("revA"),
		NewVersion("newver").Pair("revB"),
	}
	// Overwrite with new values with newer timestamps.
	{
		c.setManifestAndLock(rev, ai, newManifest, newLock)
		c.setPackageTree(rev, newPtree)
		c.setVersionMap(newPVS)
	}
	// Read new values.
	{
		gotM, gotL, ok := c.getManifestAndLock(rev, ai)
		if !ok {
			t.Error("no manifest and lock found for revision")
		}
		compareManifests(t, newManifest, gotM)
		if dl := DiffLocks(newLock, gotL); dl != nil {
			t.Errorf("lock differences:\n\t %#v", dl)
		}

		got, ok := c.getPackageTree(rev)
		if !ok {
			t.Errorf("no package tree found:\n\t(WNT): %#v", newPtree)
		}
		comparePackageTree(t, newPtree, got)

		gotV := c.getAllVersions()
		if len(gotV) != len(newPVS) {
			t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", gotV, newPVS)
		} else {
			SortPairedForDowngrade(gotV)
			for i := range newPVS {
				if !newPVS[i].identical(gotV[i]) {
					t.Errorf("unexpected versions:\n\t(GOT): %#v\n\t(WNT): %#v", gotV, newPVS)
					break
				}
			}
		}
	}
}
