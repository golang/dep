// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
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

type initTestCase struct {
	dataRoot       string
	importPaths    map[string]string
	sourceFiles    map[string]string
	goldenManifest string
	goldenLock     string
	vendorPaths    []string
}

func TestInit(t *testing.T) {
	tests := []initTestCase{

		// Both dependencies previously retrieved.  Both will show up in manifest and lock
		{
			dataRoot: "init/case1",
			importPaths: map[string]string{
				"github.com/sdboyer/deptest":    "v0.8.0",                                   // semver
				"github.com/sdboyer/deptestdos": "a0196baa11ea047dd65037287451d36b861b00ea", // random sha
			},
			sourceFiles: map[string]string{
				"thing.input.go": "foo/thing.go",
				"bar.input.go":   "foo/bar/bar.go",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "lock.golden.json",
		},

		// One dependency previously retrieved by version.  Both will show up in lock, but only retrieved one in manifest.
		{
			dataRoot: "init/case2",
			importPaths: map[string]string{
				"github.com/sdboyer/deptest": "v0.8.0", // semver
			},
			sourceFiles: map[string]string{
				"thing.input.go": "foo/thing.go",
				"bar.input.go":   "foo/bar/bar.go",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "lock.golden.json",
		},

		// One dependency previously retrieved by sha.  Both will show up in lock and manifest?
		{
			dataRoot: "init/case3",
			importPaths: map[string]string{
				"github.com/sdboyer/deptestdos": "a0196baa11ea047dd65037287451d36b861b00ea",
			},
			sourceFiles: map[string]string{
				"thing.input.go": "foo/thing.go",
				"bar.input.go":   "foo/bar/bar.go",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "lock.golden.json",
		},
	}

	runTest := func(t *testing.T, testCase initTestCase) {
		test.NeedsExternalNetwork(t)
		test.NeedsGit(t)

		h := test.NewHelper(t)
		defer h.Cleanup()

		h.TempDir("src")
		h.Setenv("GOPATH", h.Path("."))

		// checkout the specified revisions
		for ip, rev := range testCase.importPaths {
			h.RunGo("get", ip)
			repoDir := h.Path("src/" + ip)
			h.RunGit(repoDir, "checkout", rev)
		}

		// Build a fake consumer of these packages.
		root := "src/github.com/golang/notexist"
		for src, dest := range testCase.sourceFiles {
			h.TempCopy(root+"/"+dest, testCase.dataRoot+"/"+src)
		}

		h.Cd(h.Path(root))
		h.Run("init")

		wantManifest := h.GetTestFileString(testCase.dataRoot + "/" + testCase.goldenManifest)
		gotManifest := h.ReadManifest()
		if wantManifest != gotManifest {
			if *test.UpdateGolden {
				if err := h.WriteTestFile(testCase.dataRoot+"/"+testCase.goldenManifest, gotManifest); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Errorf("expected %s, got %s", wantManifest, gotManifest)
			}
		}

		wantLock := h.GetTestFileString(testCase.dataRoot + "/" + testCase.goldenLock)
		gotLock := h.ReadLock()
		if wantLock != gotLock {
			if *test.UpdateGolden {
				if err := h.WriteTestFile(testCase.dataRoot+"/"+testCase.goldenLock, gotLock); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Errorf("expected %s, got %s", wantLock, gotLock)
			}
		}

		// vendor should have been created & populated
		for ip := range testCase.importPaths {
			h.MustExist(h.Path(root + "/vendor/" + ip))
		}
	}

	for _, testCase := range tests {
		runTest(t, testCase)
	}
}
