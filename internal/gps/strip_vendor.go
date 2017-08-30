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

	if info.Name() == "vendor" {
		if _, err := os.Lstat(path); err != nil {
			return err
		}

		if (info.Mode() & os.ModeSymlink) != 0 {
			realInfo, err := os.Stat(path)
			if err != nil {
				return err
			}
			if realInfo.IsDir() {
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

	return nil
}
