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
	logger := log.New(test.Writer{TB: t}, "", 0)

	start := time.Now()
	bc, err := newBoltCache(cpath, start.Unix(), logger)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.close()
	c := bc.newSingleSourceCache(pi)

	rev := Revision("test")
	ai := ProjectAnalyzerInfo{Name: "name", Version: 42}

	manifest := &simpleRootManifest{
		c: ProjectConstraints{
			ProjectRoot("foo"): ProjectProperties{
				Constraint: Any(),
			},
			ProjectRoot("bar"): ProjectProperties{
				Source:     "whatever",
				Constraint: testSemverConstraint(t, "> 1.3"),
			},
		},
		ovr: ProjectConstraints{
			ProjectRoot("b"): ProjectProperties{
				Constraint: testSemverConstraint(t, "2.0.0"),
			},
		},
	}

	lock := &safeLock{
		h: []byte("test_hash"),
		p: []LockedProject{
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

	// Write values timestamped > `start`.
	{
		c.setManifestAndLock(rev, ai, manifest, lock)
		c.setPackageTree(rev, ptree)
		c.setVersionMap(pvs)
	}
	// Read back values timestamped > `start`.
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

		gotV, ok := c.getAllVersions()
		if !ok || len(gotV) != len(pvs) {
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

	if err := bc.close(); err != nil {
		t.Fatal("failed to close cache:", err)
	}

	// Read with a later epoch. Expect no *timestamped* values, since all were < `after`.
	{
		after := start.Add(1000 * time.Hour)
		bc, err = newBoltCache(cpath, after.Unix(), logger)
		if err != nil {
			t.Fatal(err)
		}
		c = bc.newSingleSourceCache(pi)

		gotM, gotL, ok := c.getManifestAndLock(rev, ai)
		if !ok {
			t.Error("no manifest and lock found for revision")
		}
		compareManifests(t, manifest, gotM)
		if dl := DiffLocks(lock, gotL); dl != nil {
			t.Errorf("lock differences:\n\t %#v", dl)
		}

		gotPtree, ok := c.getPackageTree(rev)
		if !ok {
			t.Errorf("no package tree found:\n\t(WNT): %#v", ptree)
		}
		comparePackageTree(t, ptree, gotPtree)

		pvs, ok := c.getAllVersions()
		if ok || len(pvs) > 0 {
			t.Errorf("expected no cached versions, but got:\n\t%#v", pvs)
		}
	}

	if err := bc.close(); err != nil {
		t.Fatal("failed to close cache:", err)
	}

	// Re-connect with the original epoch.
	bc, err = newBoltCache(cpath, start.Unix(), logger)
	if err != nil {
		t.Fatal(err)
	}
	c = bc.newSingleSourceCache(pi)
	// Read values timestamped > `start`.
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

		gotV, ok := c.getAllVersions()
		if !ok || len(gotV) != len(pvs) {
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
	newManifest := &simpleRootManifest{
		c: ProjectConstraints{
			ProjectRoot("foo"): ProjectProperties{
				Constraint: NewBranch("master"),
			},
			ProjectRoot("bar"): ProjectProperties{
				Source:     "whatever",
				Constraint: testSemverConstraint(t, "> 1.5"),
			},
		},
	}

	newLock := &safeLock{
		h: []byte("new_test_hash"),
		p: []LockedProject{
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
	// Overwrite with new values, and with timestamps > `after`.
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

		gotV, ok := c.getAllVersions()
		if !ok || len(gotV) != len(newPVS) {
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
