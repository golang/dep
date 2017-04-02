// +build windows

package gps

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// On windows, links can be symlinks (which behave like Unix symlinks, mostly)
// or 'directory junctions', which respond 'true' to os.FileInfo.IsDir(). Run
// all these tests twice: once using symlinks, and once using junctions.
func stripVendorTestCase(tc fsTestCase) func(*testing.T) {
	testcase := func(useJunctions bool) func(*testing.T) {
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

			if useJunctions {
				tc.before.setupUsingJunctions(t)
			} else {
				tc.before.setup(t)
			}

			if err := filepath.Walk(tempDir, stripVendor); err != nil {
				t.Errorf("filepath.Walk err=%q", err)
			}

			tc.after.assert(t)
		}
	}

	return func(t *testing.T) {
		t.Run("using junctions", testcase(true))
		t.Run("without junctions", testcase(false))
	}
}
