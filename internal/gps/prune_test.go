package gps

import (
	"io/ioutil"
	"log"
	"testing"

	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/test"
)

func TestPruneEmptyDirs(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir(".")

	testcases := []struct {
		name string
		fs   fsTestCase
		err  bool
	}{
		{
			"empty-dir",
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
				},
				after: filesystemState{},
			},
			false,
		},
		{
			"non-empty-dir",
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
					files: []fsPath{
						{"dir", "file"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"dir"},
					},
					files: []fsPath{
						{"dir", "file"},
					},
				},
			},
			false,
		},
		{
			"nested-empty-dirs",
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"dirs"},
						{"dirs", "dir1"},
						{"dirs", "dir2"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"dirs"},
					},
				},
			},
			false,
		},
		{
			"mixed-dirs",
			fsTestCase{
				before: filesystemState{
					dirs: []fsPath{
						{"dir1"},
						{"dir2"},
						{"dir3"},
						{"dir4"},
					},
					files: []fsPath{
						{"dir3", "file"},
						{"dir4", "file"},
					},
				},
				after: filesystemState{
					dirs: []fsPath{
						{"dir3"},
						{"dir4"},
					},
					files: []fsPath{
						{"dir3", "file"},
						{"dir4", "file"},
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

			err := pruneEmptyDirs(baseDir, logger)
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}

			tc.fs.after.assert(t)
		})
	}
}

func TestCalculateEmptyDirs(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir(".")

	testcases := []struct {
		name      string
		fs        filesystemState
		emptyDirs int
		err       bool
	}{
		{
			"empty-dir",
			filesystemState{
				dirs: []fsPath{
					{"dir"},
				},
			},
			1,
			false,
		},
		{
			"non-empty-dir",
			filesystemState{
				dirs: []fsPath{
					{"dir"},
				},
				files: []fsPath{
					{"dir", "file"},
				},
			},
			0,
			false,
		},
		{
			"nested-empty-dirs",
			filesystemState{
				dirs: []fsPath{
					{"dirs"},
					{"dirs", "dir1"},
					{"dirs", "dir2"},
				},
			},
			2,
			false,
		},
		{
			"mixed-dirs",
			filesystemState{
				dirs: []fsPath{
					{"dir1"},
					{"dir2"},
					{"dir3"},
					{"dir4"},
				},
				files: []fsPath{
					{"dir3", "file"},
					{"dir4", "file"},
				},
			},
			2,
			false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h.TempDir(tc.name)
			baseDir := h.Path(tc.name)

			tc.fs.root = baseDir
			tc.fs.setup(t)

			emptyDirs, err := calculateEmptyDirs(baseDir)
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if len(emptyDirs) != tc.emptyDirs {
				t.Fatalf("expected %d paths, got %d", tc.emptyDirs, len(emptyDirs))
			}
			for _, dir := range emptyDirs {
				if nonEmpty, err := fs.IsNonEmptyDir(dir); err != nil {
					t.Fatalf("unexpected error: %s", err)
				} else if nonEmpty {
					t.Fatalf("expected %s to be empty, but it wasn't", dir)
				}
			}
		})
	}
}
