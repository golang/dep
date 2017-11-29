// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"os"
	"path/filepath"
)

// filesystemState represents the state of a file system.
type filesystemState struct {
	root  string
	dirs  []string
	files []string
	links []fsLink
}

// fsLink represents a symbolic link.
type fsLink struct {
	path string
	to   string
}

// deriveFilesystemState returns a filesystemState based on the state of
// the filesystem on root.
func deriveFilesystemState(root string) (filesystemState, error) {
	fs := filesystemState{
		root: root,
	}

	err := filepath.Walk(fs.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == fs.root {
			return nil
		}

		relPath, err := filepath.Rel(fs.root, path)
		if err != nil {
			return err
		}

		if (info.Mode() & os.ModeSymlink) != 0 {
			eval, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}

			fs.links = append(fs.links, fsLink{
				path: relPath,
				to:   eval,
			})

			return nil
		}

		if info.IsDir() {
			fs.dirs = append(fs.dirs, relPath)

			return nil
		}

		fs.files = append(fs.files, relPath)

		return nil
	})

	if err != nil {
		return filesystemState{}, err
	}

	return fs, nil
}
