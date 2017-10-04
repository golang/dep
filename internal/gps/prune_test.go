// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"io/ioutil"
	"log"
	"testing"

	"github.com/golang/dep/internal/test"
)

func TestPruneUnusedPackages(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir(".")

	pr := "github.com/test/project"
	pi := ProjectIdentifier{ProjectRoot: ProjectRoot(pr)}

	testcases := []struct {
		name string
		lp   LockedProject
		fs   fsTestCase
		err  bool
	}{
		{
			"one-package",
			LockedProject{
				pi:   pi,
				pkgs: []string{"."},
			},
			fsTestCase{
				before: filesystemState{
					files: []fsPath{
						{"main.go"},
					},
				},
				after: filesystemState{
					files: []fsPath{
						{"main.go"},
					},
				},
			},
			false,
		},
		{
			"nested-package",
			LockedProject{
				pi:   pi,
				pkgs: []string{"pkg"},
			},
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"pkg"},
					},
					files: []fsPath{
						{"main.go"},
						{"pkg", "main.go"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"pkg"},
					},
					files: []fsPath{
						{"pkg", "main.go"},
					},
				},
			},
			false,
		},
		{
			"complex-project",
			LockedProject{
				pi:   pi,
				pkgs: []string{"pkg", "pkg/nestedpkg/otherpkg"},
			},
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"pkg"},
						{"pkg", "nestedpkg"},
						{"pkg", "nestedpkg", "otherpkg"},
					},
					files: []fsPath{
						{"main.go"},
						{"COPYING"},
						{"pkg", "main.go"},
						{"pkg", "nestedpkg", "main.go"},
						{"pkg", "nestedpkg", "PATENT.md"},
						{"pkg", "nestedpkg", "otherpkg", "main.go"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"pkg"},
						{"pkg", "nestedpkg"},
						{"pkg", "nestedpkg", "otherpkg"},
					},
					files: []fsPath{
						{"COPYING"},
						{"pkg", "main.go"},
						{"pkg", "nestedpkg", "PATENT.md"},
						{"pkg", "nestedpkg", "otherpkg", "main.go"},
					},
				},
			},
			false,
		},
	}

	logger := log.New(ioutil.Discard, "", 0)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h.TempDir(pr)
			projectDir := h.Path(pr)
			tc.fs.before.root = projectDir
			tc.fs.after.root = projectDir

			tc.fs.before.setup(t)

			err := pruneUnusedPackages(tc.lp, projectDir, logger)
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			tc.fs.after.assert(t)
		})
	}
}

func TestPruneNonGoFiles(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir(".")

	testcases := []struct {
		name string
		fs   fsTestCase
		err  bool
	}{
		{
			"one-file",
			fsTestCase{
				before: filesystemState{
					files: []fsPath{
						{"README.md"},
					},
				},
				after: filesystemState{},
			},
			false,
		},
		{
			"multiple-files",
			fsTestCase{
				before: filesystemState{
					files: []fsPath{
						{"main.go"},
						{"main_test.go"},
						{"README"},
					},
				},
				after: filesystemState{
					files: []fsPath{
						{"main.go"},
						{"main_test.go"},
					},
				},
			},
			false,
		},
		{
			"mixed-files",
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
					files: []fsPath{
						{"dir", "main.go"},
						{"dir", "main_test.go"},
						{"dir", "db.sqlite"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
					files: []fsPath{
						{"dir", "main.go"},
						{"dir", "main_test.go"},
					},
				},
			},
			false,
		},
	}

	logger := log.New(ioutil.Discard, "", 0)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h.TempDir(tc.name)
			baseDir := h.Path(tc.name)
			tc.fs.before.root = baseDir
			tc.fs.after.root = baseDir

			tc.fs.before.setup(t)

			err := pruneNonGoFiles(baseDir, logger)
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			tc.fs.after.assert(t)
		})
	}
}

func TestPruneGoTestFiles(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir(".")

	testcases := []struct {
		name string
		fs   fsTestCase
		err  bool
	}{
		{
			"one-test-file",
			fsTestCase{
				before: filesystemState{
					files: []fsPath{
						{"main_test.go"},
					},
				},
				after: filesystemState{},
			},
			false,
		},
		{
			"multiple-files",
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
					files: []fsPath{
						{"dir", "main_test.go"},
						{"dir", "main2_test.go"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
				},
			},
			false,
		},
		{
			"mixed-files",
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
					files: []fsPath{
						{"dir", "main.go"},
						{"dir", "main2.go"},
						{"dir", "main_test.go"},
						{"dir", "main2_test.go"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
					files: []fsPath{
						{"dir", "main.go"},
						{"dir", "main2.go"},
					},
				},
			},
			false,
		},
	}

	logger := log.New(ioutil.Discard, "", 0)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h.TempDir(tc.name)
			baseDir := h.Path(tc.name)
			tc.fs.before.root = baseDir
			tc.fs.after.root = baseDir

			tc.fs.before.setup(t)

			err := pruneGoTestFiles(baseDir, logger)
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			tc.fs.after.assert(t)
		})
	}
}
