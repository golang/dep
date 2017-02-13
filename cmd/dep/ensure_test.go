// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/test"
	"github.com/sdboyer/gps"
)

func TestEnsureOverrides(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))

	h.TempCopy("src/thing/thing.go", "ensure/overrides_main.go")
	h.Cd(h.Path("src/thing"))

	h.Run("init")
	h.Run("ensure", "-override", "github.com/carolynvs/go-dep-test@0.1.1")

	goldenManifest := "ensure/overrides_manifest.golden.json"
	wantManifest := h.GetTestFileString(goldenManifest)
	gotManifest := h.ReadManifest()
	if gotManifest != wantManifest {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenManifest, string(gotManifest)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s, got %s", wantManifest, gotManifest)
		}
	}

	goldenLock := "ensure/overrides_lock.golden.json"
	wantLock := h.GetTestFileString(goldenLock)
	gotLock := h.ReadLock()
	if gotLock != wantLock {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenLock, string(gotLock)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s, got %s", wantLock, gotLock)
		}
	}
}

func TestEnsureEmptyRepoNoArgs(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))
	h.TempCopy("src/thing/thing.go", "ensure/bare_main.go")
	h.Cd(h.Path("src/thing"))

	h.Run("init")
	h.Run("ensure")

	// make sure vendor exists
	h.MustExist(h.Path("src/thing/vendor/github.com/jimmysmith95/fixed-version"))

	goldenManifest := "ensure/bare_manifest.golden.json"
	wantManifest := h.GetTestFileString(goldenManifest)
	gotManifest := h.ReadManifest()
	if gotManifest != wantManifest {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenManifest, string(gotManifest)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s, got %s", wantManifest, gotManifest)
		}
	}

	goldenLock := "ensure/bare_lock.golden.json"
	wantLock := h.GetTestFileString(goldenLock)
	gotLock := h.ReadLock()
	if gotLock != wantLock {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenLock, string(gotLock)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s, got %s", wantLock, gotLock)
		}
	}
}

func TestDeduceConstraint(t *testing.T) {
	sv, err := gps.NewSemverConstraint("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	constraints := map[string]gps.Constraint{
		"v1.2.3": sv,
		"5b3352dc16517996fb951394bcbbe913a2a616e3": gps.Revision("5b3352dc16517996fb951394bcbbe913a2a616e3"),

		// valid bzr revs
		"jess@linux.com-20161116211307-wiuilyamo9ian0m7": gps.Revision("jess@linux.com-20161116211307-wiuilyamo9ian0m7"),

		// invalid bzr revs
		"go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn": gps.NewVersion("go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn"),
		"go4@golang.org-sadfasdf-":                  gps.NewVersion("go4@golang.org-sadfasdf-"),
		"20120425195858-psty8c35ve2oej8t":           gps.NewVersion("20120425195858-psty8c35ve2oej8t"),
	}

	for str, expected := range constraints {
		c := deduceConstraint(str)
		if c != expected {
			t.Fatalf("expected: %#v, got %#v for %s", expected, c, str)
		}
	}
}

func TestEnsureUpdate(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	// Setup up a test project
	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))
	h.TempCopy("src/thing/main.go", "ensure/update_main.go")
	origManifest := "ensure/update_manifest.json"
	h.TempCopy("src/thing/manifest.json", origManifest)
	origLock := "ensure/update_lock.json"
	h.TempCopy("src/thing/lock.json", origLock)
	h.Cd(h.Path("src/thing"))

	h.Run("ensure", "-update", "github.com/carolynvs/go-dep-test")

	// Verify that the manifest wasn't modified by -update
	wantManifest := h.GetTestFileString(origManifest)
	gotManifest := h.ReadManifest()
	if gotManifest != wantManifest {
		t.Fatalf("The manifest should not be modified during an update. Expected %s, got %s", origManifest, gotManifest)
	}

	// Verify the lock matches the expected golden file
	goldenLock := "ensure/update_lock.golden.json"
	wantLock := h.GetTestFileString(goldenLock)
	gotLock := h.ReadLock()
	if gotLock != wantLock {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenLock, string(gotLock)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s, got %s", wantLock, gotLock)
		}
	}
}
