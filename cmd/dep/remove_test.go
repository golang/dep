// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"path/filepath"
	"testing"

	"github.com/golang/dep/test"
)

type removeTestCase struct {
	dataRoot       string
	command        []string
	importPaths    map[string]string
	sourceFiles    map[string]string
	goldenManifest string
	goldenLock     string
	vendorPaths    []string
}

func TestRemove(t *testing.T) {
	tests := []removeTestCase{
		{
			dataRoot: "remove/case0",
			command:  []string{"remove", "-unused"},
			importPaths: map[string]string{
				"github.com/sdboyer/deptest":    "v0.8.0",                                   // semver
				"github.com/sdboyer/deptestdos": "a0196baa11ea047dd65037287451d36b861b00ea", // random sha
			},
			sourceFiles: map[string]string{
				"main.go":             "main.go",
				"manifest.input.json": "manifest.json",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "",
		},
		{
			dataRoot: "remove/case1",
			command:  []string{"remove", "github.com/not/used"},
			importPaths: map[string]string{
				"github.com/sdboyer/deptest":    "v0.8.0",                                   // semver
				"github.com/sdboyer/deptestdos": "a0196baa11ea047dd65037287451d36b861b00ea", // random sha
			},
			sourceFiles: map[string]string{
				"main.go":             "main.go",
				"manifest.input.json": "manifest.json",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "",
		},
		{
			dataRoot: "remove/case2",
			command:  []string{"remove", "-force", "github.com/sdboyer/deptestdos", "github.com/not/used"},
			importPaths: map[string]string{
				"github.com/sdboyer/deptest":    "v0.8.0",                                   // semver
				"github.com/sdboyer/deptestdos": "a0196baa11ea047dd65037287451d36b861b00ea", // random sha
			},
			sourceFiles: map[string]string{
				"main.go":             "main.go",
				"manifest.input.json": "manifest.json",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "lock.golden.json",
		},
	}

	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	for _, testCase := range tests {
		t.Run(testCase.dataRoot, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()

			h.TempDir("src")
			h.Setenv("GOPATH", h.Path("."))

			// checkout the specified revisions
			for ip, rev := range testCase.importPaths {
				h.RunGo("get", ip)
				repoDir := h.Path(filepath.Join("src", ip))
				h.RunGit(repoDir, "checkout", rev)
			}

			// Build a fake consumer of these packages.
			root := "src/github.com/golang/notexist"
			for src, dest := range testCase.sourceFiles {
				h.TempCopy(filepath.Join(root, dest), filepath.Join(testCase.dataRoot, src))
			}

			h.Cd(h.Path(root))
			h.Run(testCase.command...)

			wantPath := filepath.Join(testCase.dataRoot, testCase.goldenManifest)
			wantManifest := h.GetTestFileString(wantPath)
			gotManifest := h.ReadManifest()
			if wantManifest != gotManifest {
				if *test.UpdateGolden {
					if err := h.WriteTestFile(wantPath, gotManifest); err != nil {
						t.Fatal(err)
					}
				} else {
					t.Errorf("expected %s, got %s", wantManifest, gotManifest)
				}
			}

			if testCase.goldenLock != "" {
				wantPath = filepath.Join(testCase.dataRoot, testCase.goldenLock)
				wantLock := h.GetTestFileString(wantPath)
				gotLock := h.ReadLock()
				if wantLock != gotLock {
					if *test.UpdateGolden {
						if err := h.WriteTestFile(wantPath, gotLock); err != nil {
							t.Fatal(err)
						}
					} else {
						t.Errorf("expected %s, got %s", wantLock, gotLock)
					}
				}
			}
		})
	}
}

func TestRemoveErrors(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))

	// Build a fake consumer of these packages.
	sourceFiles := map[string]string{
		"main.go":             "main.go",
		"manifest.input.json": "manifest.json",
	}
	root := "src/github.com/golang/notexist"
	for src, dest := range sourceFiles {
		h.TempCopy(filepath.Join(root, dest), filepath.Join("remove/case0", src))
	}

	h.Cd(h.Path(root))

	if err := h.DoRun([]string{"remove", "-unused", "github.com/not/used"}); err == nil {
		t.Fatal("rm with both -unused and arg should have failed")
	}

	if err := h.DoRun([]string{"remove", "github.com/not/present"}); err == nil {
		t.Fatal("rm with arg not in manifest should have failed")
	}

	if err := h.DoRun([]string{"remove", "github.com/not/used", "github.com/not/present"}); err == nil {
		t.Fatal("rm with one arg not in manifest should have failed")
	}

	if err := h.DoRun([]string{"remove", "github.com/sdboyer/deptest"}); err == nil {
		t.Fatal("rm of arg in manifest and imports should have failed without -force")
	}
}
