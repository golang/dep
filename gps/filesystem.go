// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"os"
	"path/filepath"
	"runtime"
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

		symlink := (info.Mode() & os.ModeSymlink) == os.ModeSymlink
		dir := info.IsDir()

		if runtime.GOOS == "windows" && symlink && dir {
			// This could be a Windows junction directory. Support for these in the
			// standard library is spotty, and we could easily delete an important
			// folder if we called os.Remove or os.RemoveAll. Just skip these.
			//
			// TODO: If we could distinguish between junctions and Windows symlinks,
			// we might be able to safely delete symlinks, even though junctions are
			// dangerous.

			return nil
		}

		if symlink {
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

		if dir {
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
