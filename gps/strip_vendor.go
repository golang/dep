// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//+build !windows

package gps

import (
	"os"
	"path/filepath"
)

func stripVendor(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	// Skip anything not named vendor
	if info.Name() != "vendor" {
		return nil
	}

	// If the file is a symlink to a directory, delete the symlink.
	if (info.Mode() & os.ModeSymlink) != 0 {
		if realInfo, err := os.Stat(path); err == nil && realInfo.IsDir() {
			return os.Remove(path)
		}
	}

	if info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		return filepath.SkipDir
	}

	return nil
}
