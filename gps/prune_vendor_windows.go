// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"os"
	"path/filepath"
)

func collectNestedVendorDirs(fsState filesystemState) []string {
	toDelete := make([]string, 0, len(fsState.dirs)/4)

	for _, dir := range fsState.dirs {
		if filepath.Base(dir) == "vendor" {
			toDelete = append(toDelete, filepath.Join(fsState.root, dir))
		}
	}

	for _, link := range fsState.links {
		if filepath.Base(link.path) == "vendor" {
			// This could be a windows junction directory. Support for these in the
			// standard library is spotty, and we could easily delete an important
			// folder if we called os.Remove or os.RemoveAll. Just skip these.
			//
			// TODO: If we could distinguish between junctions and Windows symlinks,
			// we might be able to safely delete symlinks, even though junctions are
			// dangerous.
			if info, err := os.Stat(link.path); err == nil && info.IsDir() {
				toDelete = append(toDelete, filepath.Join(fsState.root, link.path))
			}

		}
	}

	return toDelete
}
