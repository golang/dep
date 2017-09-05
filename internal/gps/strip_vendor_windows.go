// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"fmt"
	"os"
	"path/filepath"
)

func stripVendor(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if info.Name() == "vendor" {
		if _, err := os.Lstat(path); err == nil {
			symlink := (info.Mode() & os.ModeSymlink) != 0
			dir := info.IsDir()

			switch {
			case symlink && dir:
				// This could be a windows junction directory. Support for these in the
				// standard library is spotty, and we could easily delete an important
				// folder if we called os.Remove or os.RemoveAll. Just skip these.
				//
				// TODO: If we could distinguish between junctions and Windows symlinks,
				// we might be able to safely delete symlinks, even though junctions are
				// dangerous.
				return filepath.SkipDir

			case symlink:
				realInfo, err := os.Stat(path)
				if err != nil {
					return err
				}
				if realInfo.IsDir() {
					return os.Remove(path)
				}

			case dir:
				if err := os.RemoveAll(path); err != nil {
					fmt.Printf("removing %s failed: %s, skipping. Please check manually\n", path, err)
				}
				return filepath.SkipDir
			}
		}
	}

	return nil
}
