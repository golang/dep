// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"

	"github.com/golang/dep/test"
)

func TestContains(t *testing.T) {
	a := []string{"a", "b", "abcd"}

	if !contains(a, "a") {
		t.Fatal("expected array to contain 'a'")
	}
	if contains(a, "d") {
		t.Fatal("expected array to not contain 'd'")
	}
}

func TestIsStdLib(t *testing.T) {
	tests := map[string]bool{
		"github.com/Sirupsen/logrus": false,
		"encoding/json":              true,
		"golang.org/x/net/context":   false,
		"net/context":                true,
		".":                          false,
	}

	for p, e := range tests {
		b := isStdLib(p)
		if b != e {
			t.Fatalf("%s: expected %t got %t", p, e, b)
		}
	}
}

func TestInit(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))

	importPaths := map[string]string{
		"github.com/pkg/errors":      "v0.8.0",                                   // semver
		"github.com/Sirupsen/logrus": "42b84f9ec624953ecbf81a94feccb3f5935c5edf", // random sha
	}

	// checkout the specified revisions
	for ip, rev := range importPaths {
		h.RunGo("get", ip)
		repoDir := h.Path("src/" + ip)
		h.RunGit(repoDir, "checkout", rev)
	}

	// Build a fake consumer of these packages.
	root := "src/github.com/golang/notexist"
	h.TempCopy(root+"/foo/thing.go", "init/thing.input.go")
	h.TempCopy(root+"/foo/bar/bar.go", "init/bar.input.go")

	h.Cd(h.Path(root))
	h.Run("init")

	goldenManifest := "init/manifest.golden.json"
	wantManifest := h.GetTestFileString(goldenManifest)
	gotManifest := h.ReadManifest()
	if wantManifest != gotManifest {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenManifest, gotManifest); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("expected %s, got %s", wantManifest, gotManifest)
		}
	}

	sysCommit := h.GetCommit("go.googlesource.com/sys")
	goldenLock := "init/lock.golden.json"
	wantLock := strings.Replace(h.GetTestFileString(goldenLock), "` + sysCommit + `", sysCommit, 1)
	gotLock := h.ReadLock()
	if wantLock != gotLock {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenLock, gotLock); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("expected %s, got %s", wantLock, gotLock)
		}
	}

	// vendor should have been created & populated
	h.MustExist(h.Path(root + "/vendor/github.com/Sirupsen/logrus"))
}
