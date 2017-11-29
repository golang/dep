// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//+build !windows

package gps

import (
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
			toDelete = append(toDelete, filepath.Join(fsState.root, link.path))
		}
	}

	return toDelete
}
