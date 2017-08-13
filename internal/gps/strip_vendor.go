// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//+build !windows

package gps

import (
	"os"
	"path/filepath"
)

func stripNestedVendorDirs(baseDir string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		// Ignore anything that's not named "vendor".
		if info.Name() != "vendor" {
			return nil
		}

		// Ignore the base vendor directory.
		if path == baseDir {
			return nil
		}

		// If it's a directory, delete it along with its content.
		if info.IsDir() {
			return removeAll(path)
		}

		if _, err := os.Lstat(path); err != nil {
			return nil
		}

		// If it is a symlink, check if the target is a directory and delete that instead.
		if (info.Mode() & os.ModeSymlink) != 0 {
			realInfo, err := os.Stat(path)
			if err != nil {
				return err
			}
			if realInfo.IsDir() {
				return os.Remove(path)
			}
		}

		return nil
	}
}
