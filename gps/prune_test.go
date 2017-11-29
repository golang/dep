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

func TestRootPruneOptions_PruneOptionsFor(t *testing.T) {
	pr := ProjectRoot("github.com/golang/dep")

	o := RootPruneOptions{
		PruneOptions: PruneNestedVendorDirs,
		ProjectOptions: PruneProjectOptions{
			pr: PruneGoTestFiles,
		},
	}

	if (o.PruneOptionsFor(pr) & PruneGoTestFiles) != PruneGoTestFiles {
		t.Fatalf("invalid prune options.\n\t(GOT): %d\n\t(WNT): %d", o.PruneOptionsFor(pr), PruneGoTestFiles)
	}
}

func TestPruneProject(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	pr := "github.com/project/repository"
	h.TempDir(pr)

	baseDir := h.Path(".")
	lp := LockedProject{
		pi: ProjectIdentifier{
			ProjectRoot: ProjectRoot(pr),
		},
		pkgs: []string{},
	}

	options := PruneNestedVendorDirs | PruneNonGoFiles | PruneGoTestFiles | PruneUnusedPackages
	logger := log.New(ioutil.Discard, "", 0)

	err := PruneProject(baseDir, lp, options, logger)
	if err != nil {
		t.Fatal(err)
	}
}

// func TestPruneVendorDirs(t *testing.T) {
// 	h := test.NewHelper(t)
// 	defer h.Cleanup()

// 	h.TempDir(".")
// 	baseDir := h.Path(".")

// 	err := pruneVendorDirs(baseDir)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// }

func TestPruneUnusedPackages(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir(".")

	pr := "github.com/sample/repository"
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
				pi: pi,
				pkgs: []string{
					".",
				},
			},
			fsTestCase{
				before: filesystemState{
					files: []string{
						"main.go",
					},
				},
				after: filesystemState{
					files: []string{
						"main.go",
					},
				},
			},
			false,
		},
		{
			"nested-package",
			LockedProject{
				pi: pi,
				pkgs: []string{
					"pkg",
				},
			},
			fsTestCase{
				before: filesystemState{
					dirs: []string{
						"pkg",
					},
					files: []string{
						"main.go",
						"pkg/main.go",
					},
				},
				after: filesystemState{
					dirs: []string{
						"pkg",
					},
					files: []string{
						"pkg/main.go",
					},
				},
			},
			false,
		},
		{
			"complex-project",
			LockedProject{
				pi: pi,
				pkgs: []string{
					"pkg",
					"pkg/nestedpkg/otherpkg",
				},
			},
			fsTestCase{
				before: filesystemState{
					dirs: []string{
						"pkg",
						"pkg/nestedpkg",
						"pkg/nestedpkg/otherpkg",
					},
					files: []string{
						"main.go",
						"COPYING",
						"pkg/main.go",
						"pkg/nestedpkg/main.go",
						"pkg/nestedpkg/PATENT.md",
						"pkg/nestedpkg/otherpkg/main.go",
					},
				},
				after: filesystemState{
					dirs: []string{
						"pkg",
						"pkg/nestedpkg",
						"pkg/nestedpkg/otherpkg",
					},
					files: []string{
						"COPYING",
						"pkg/main.go",
						"pkg/nestedpkg/PATENT.md",
						"pkg/nestedpkg/otherpkg/main.go",
					},
				},
			},
			false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h.TempDir(pr)
			baseDir := h.Path(pr)
			tc.fs.before.root = baseDir
			tc.fs.after.root = baseDir
			tc.fs.setup(t)

			fs, err := deriveFilesystemState(baseDir)
			if err != nil {
				t.Fatal(err)
			}

			_, err = pruneUnusedPackages(tc.lp, fs)
			if tc.err && err == nil {
				t.Fatalf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			tc.fs.assert(t)
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
					files: []string{
						"README.md",
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
					files: []string{
						"main.go",
						"main_test.go",
						"README",
					},
				},
				after: filesystemState{
					files: []string{
						"main.go",
						"main_test.go",
					},
				},
			},
			false,
		},
		{
			"mixed-files",
			fsTestCase{
				before: filesystemState{
					dirs: []string{
						"dir",
					},
					files: []string{
						"dir/main.go",
						"dir/main_test.go",
						"dir/db.sqlite",
					},
				},
				after: filesystemState{
					dirs: []string{
						"dir",
					},
					files: []string{
						"dir/main.go",
						"dir/main_test.go",
					},
				},
			},
			false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h.TempDir(tc.name)
			baseDir := h.Path(tc.name)
			tc.fs.before.root = baseDir
			tc.fs.after.root = baseDir

			tc.fs.setup(t)

			fs, err := deriveFilesystemState(baseDir)
			if err != nil {
				t.Fatal(err)
			}

			err = pruneNonGoFiles(fs)
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			tc.fs.assert(t)
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
					files: []string{
						"main_test.go",
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
					dirs: []string{
						"dir",
					},
					files: []string{
						"dir/main_test.go",
						"dir/main2_test.go",
					},
				},
				after: filesystemState{
					dirs: []string{
						"dir",
					},
				},
			},
			false,
		},
		{
			"mixed-files",
			fsTestCase{
				before: filesystemState{
					dirs: []string{
						"dir",
					},
					files: []string{
						"dir/main.go",
						"dir/main2.go",
						"dir/main_test.go",
						"dir/main2_test.go",
					},
				},
				after: filesystemState{
					dirs: []string{
						"dir",
					},
					files: []string{
						"dir/main.go",
						"dir/main2.go",
					},
				},
			},
			false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h.TempDir(tc.name)
			baseDir := h.Path(tc.name)
			tc.fs.before.root = baseDir
			tc.fs.after.root = baseDir

			tc.fs.setup(t)

			fs, err := deriveFilesystemState(baseDir)
			if err != nil {
				t.Fatal(err)
			}

			err = pruneGoTestFiles(fs)
			if tc.err && err == nil {
				t.Fatalf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			tc.fs.assert(t)
		})
	}
}
