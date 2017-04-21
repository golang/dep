package gps

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func stripVendorTestCase(tc fsTestCase) func(*testing.T) {
	return func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "TestStripVendor")
		if err != nil {
			t.Fatalf("ioutil.TempDir err=%q", err)
		}
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Errorf("os.RemoveAll(%q) err=%q", tempDir, err)
			}
		}()
		tc.before.root = tempDir
		tc.after.root = tempDir

		tc.before.setup(t)

		if err := filepath.Walk(tempDir, stripVendor); err != nil {
			t.Errorf("filepath.Walk err=%q", err)
		}

		tc.after.assert(t)
	}
}

func TestStripVendorDirectory(t *testing.T) {
	t.Run("vendor directory", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
				fsPath{"package", "vendor"},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
			},
		},
	}))

	t.Run("vendor file", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
			},
			files: []fsPath{
				fsPath{"package", "vendor"},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
			},
			files: []fsPath{
				fsPath{"package", "vendor"},
			},
		},
	}))
}
