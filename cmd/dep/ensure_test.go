// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"path/filepath"
	"testing"

	"github.com/golang/dep/test"
	"github.com/sdboyer/gps"
)

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

type ensureTestCase struct {
	dataRoot       string
	commands       [][]string
	sourceFiles    map[string]string
	goldenManifest string
	goldenLock     string
}

func TestEnsureCases(t *testing.T) {
	tests := []ensureTestCase{

		// Override test case
		{
			dataRoot: "ensure/override",
			commands: [][]string{
				{"init"},
				{"ensure", "-override", "github.com/sdboyer/deptest@1.0.0"},
			},
			sourceFiles: map[string]string{
				"main.go": "thing.go",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "lock.golden.json",
		},

		// Empty repo test case
		{
			dataRoot: "ensure/empty",
			commands: [][]string{
				{"init"},
				{"ensure"},
			},
			sourceFiles: map[string]string{
				"main.go": "thing.go",
			},
			goldenManifest: "manifest.golden.json",
			goldenLock:     "lock.golden.json",
		},

		// Update test case
		{
			dataRoot: "ensure/update",
			commands: [][]string{
				{"ensure", "-update", "github.com/carolynvs/go-dep-test"},
			},
			sourceFiles: map[string]string{
				"main.go":       "thing.go",
				"manifest.json": "manifest.json",
				"lock.json":     "lock.json",
			},
			goldenManifest: "manifest.json",
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

			// Build a fake consumer of these packages.
			root := "src/thing"
			for src, dest := range testCase.sourceFiles {
				h.TempCopy(filepath.Join(root, dest), filepath.Join(testCase.dataRoot, src))
			}
			h.Cd(h.Path(root))

			for _, cmd := range testCase.commands {
				h.Run(cmd...)
			}

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
		})
	}
}
