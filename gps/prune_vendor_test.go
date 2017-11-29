// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"io/ioutil"
	"os"
	"testing"
)

func pruneVendorDirsTestCase(tc fsTestCase) func(*testing.T) {
	return func(t *testing.T) {
		tempDir, err := ioutil.TempDir("", "pruneVendorDirsTestCase")
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

		tc.setup(t)

		fs, err := deriveFilesystemState(tempDir)
		if err != nil {
			t.Fatalf("deriveFilesystemState failed: %s", err)
		}

		if err := pruneVendorDirs(fs); err != nil {
			t.Errorf("pruneVendorDirs err=%q", err)
		}

		tc.assert(t)
	}
}

func TestPruneVendorDirs(t *testing.T) {
	t.Run("vendor directory", pruneVendorDirsTestCase(fsTestCase{
		before: filesystemState{
			dirs: []string{
				"package",
				"package/vendor",
			},
		},
		after: filesystemState{
			dirs: []string{
				"package",
			},
		},
	}))

	t.Run("vendor file", pruneVendorDirsTestCase(fsTestCase{
		before: filesystemState{
			dirs: []string{
				"package",
			},
			files: []string{
				"package/vendor",
			},
		},
		after: filesystemState{
			dirs: []string{
				"package",
			},
			files: []string{
				"package/vendor",
			},
		},
	}))
}
