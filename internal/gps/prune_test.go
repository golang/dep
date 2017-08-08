package gps

import (
	"io/ioutil"
	"log"
	"testing"

	"github.com/golang/dep/internal/test"
)

func TestPruneEmptyDirs(t *testing.T) {
	h := test.NewHelper(t)
	h.TempDir("baseDir1/empty-dir")

	h.TempDir("baseDir2/non-empty-dir")
	h.TempFile("baseDir2/non-empty-dir/file", "")

	h.TempDir("baseDir3/nested-empty-dirs")
	h.TempDir("baseDir3/nested-empty-dirs/empty-dir1")
	h.TempDir("baseDir3/nested-empty-dirs/empty-dir2")

	h.TempDir("baseDir4")
	h.TempDir("baseDir4/empty-dir1")
	h.TempDir("baseDir4/empty-dir2")
	h.TempDir("baseDir4/non-empty-dir1")
	h.TempFile("baseDir4/non-empty-dir1/file", "")
	h.TempDir("baseDir4/non-empty-dir2")
	h.TempFile("baseDir4/non-empty-dir2/file", "")

	testcases := []struct {
		name      string
		baseDir   string
		emptyDirs []string
		err       bool
	}{
		{"1 empty dir", h.Path("baseDir1"), []string{
			h.Path("baseDir1/empty-dir"),
		}, false},
		{"1 non-empty dir", h.Path("baseDir2"), []string{}, false},
		{"nested empty dirs", h.Path("baseDir3"), []string{
			h.Path("baseDir3/nested-empty-dirs/empty-dir1"),
			h.Path("baseDir3/nested-empty-dirs/empty-dir2"),
		}, false},
		{"mixed dirs", h.Path("baseDir4"), []string{
			h.Path("baseDir4/empty-dir1"),
		}, false},
	}

	logger := log.New(ioutil.Discard, "", 0)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := pruneEmptyDirs(tc.baseDir, logger)
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			for _, path := range tc.emptyDirs {
				h.ShouldNotExist(path)
			}
		})
	}
}

func TestCalculateEmptyDirs(t *testing.T) {
	h := test.NewHelper(t)

	h.TempDir("baseDir1/empty-dir")

	h.TempDir("baseDir2/non-empty-dir")
	h.TempFile("baseDir2/non-empty-dir/file", "")

	h.TempDir("baseDir3/nested-empty-dirs")
	h.TempDir("baseDir3/nested-empty-dirs/empty-dir1")
	h.TempDir("baseDir3/nested-empty-dirs/empty-dir2")

	h.TempDir("baseDir4")
	h.TempDir("baseDir4/empty-dir1")
	h.TempDir("baseDir4/empty-dir2")
	h.TempDir("baseDir4/non-empty-dir1")
	h.TempFile("baseDir4/non-empty-dir1/file", "")
	h.TempDir("baseDir4/non-empty-dir2")
	h.TempFile("baseDir4/non-empty-dir2/file", "")

	testcases := []struct {
		name      string
		baseDir   string
		emptyDirs []string
		err       bool
	}{
		{"1 empty dir", h.Path("baseDir1"), []string{
			h.Path("baseDir1/empty-dir"),
		}, false},
		{"1 non-empty dir", h.Path("baseDir2"), []string{}, false},
		{"nested empty dirs", h.Path("baseDir3"), []string{
			h.Path("baseDir3/nested-empty-dirs/empty-dir1"),
			h.Path("baseDir3/nested-empty-dirs/empty-dir2"),
		}, false},
		{"mixed dirs", h.Path("baseDir4"), []string{
			h.Path("baseDir4/empty-dir1"),
			h.Path("baseDir4/empty-dir2"),
		}, false},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			emptyDirs, err := calculateEmptyDirs(tc.baseDir)
			if len(emptyDirs) != len(tc.emptyDirs) {
				t.Fatalf("expected %d paths, got %d", len(tc.emptyDirs), len(emptyDirs))
			}
			for i := range tc.emptyDirs {
				if tc.emptyDirs[i] != emptyDirs[i] {
					t.Fatalf("expected %s to exists in the list, got %s", tc.emptyDirs[i], emptyDirs)
				}
			}
			if tc.err && err == nil {
				t.Errorf("expected an error, got nil")
			} else if !tc.err && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		})
	}
}
