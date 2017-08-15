// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkgtree

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// DirWalk is the type of the function called for each file system node visited
// by DirWalk. The path argument contains the argument to DirWalk as a prefix;
// that is, if DirWalk is called with "dir", which is a directory containing the
// file "a", the walk function will be called with the argument "dir/a", using
// the correct os.PathSeparator for the Go Operating System architecture,
// GOOS. The info argument is the os.FileInfo for the named path.
//
// If there was a problem walking to the file or directory named by path, the
// incoming error will describe the problem and the function can decide how to
// handle that error (and DirWalk will not descend into that directory). If an
// error is returned, processing stops. The sole exception is when the function
// returns the special value filepath.SkipDir. If the function returns
// filepath.SkipDir when invoked on a directory, DirWalk skips the directory's
// contents entirely. If the function returns filepath.SkipDir when invoked on a
// non-directory file system node, DirWalk skips the remaining files in the
// containing directory.
type DirWalkFunc func(osPathname string, info os.FileInfo, err error) error

// DirWalk walks the file tree rooted at osDirname, calling for each file system
// node in the tree, including root. All errors that arise visiting nodes are
// filtered by walkFn. The nodes are walked in lexical order, which makes the
// output deterministic but means that for very large directories DirWalk can be
// inefficient. Unlike filepath.Walk, DirWalk does follow symbolic links.
func DirWalk(osDirname string, walkFn DirWalkFunc) error {
	osDirname = filepath.Clean(osDirname)

	// Ensure parameter is a directory
	fi, err := os.Stat(osDirname)
	if err != nil {
		return errors.Wrap(err, "cannot Stat")
	}
	if !fi.IsDir() {
		return errors.Errorf("cannot verify non directory: %q", osDirname)
	}

	// Initialize a work queue with the empty string, which signifies the
	// starting directory itself.
	queue := []string{""}

	var osRelative string // os-specific relative pathname under dirname

	// As we enumerate over the queue and encounter a directory, its children
	// will be added to the work queue.
	for len(queue) > 0 {
		// Unshift a pathname from the queue (breadth-first traversal of
		// hierarchy)
		osRelative, queue = queue[0], queue[1:]
		osPathname := filepath.Join(osDirname, osRelative)

		fi, err = os.Lstat(osPathname)
		err = walkFn(osPathname, fi, errors.Wrap(err, "cannot Lstat"))
		if err != nil {
			if err == filepath.SkipDir {
				// We have lstat, and we need stat right now
				fi, err = os.Stat(osPathname)
				if err != nil {
					return errors.Wrap(err, "cannot Stat")
				}
				if !fi.IsDir() {
					// Consume items from queue while they have the same parent
					// as the current item.
					osParent := filepath.Dir(osPathname)
					for len(queue) > 0 && strings.HasPrefix(queue[0], osParent) {
						log.Printf("skipping sibling: %s", queue[0])
						queue = queue[1:]
					}
				}

				continue
			}
			return errors.Wrap(err, "DirWalkFunction")
		}

		if fi.IsDir() {
			osChildrenNames, err := sortedChildrenFromDirname(osPathname)
			if err != nil {
				return errors.Wrap(err, "cannot get list of directory children")
			}
			for _, osChildName := range osChildrenNames {
				switch osChildName {
				case ".", "..":
					// skip
				default:
					queue = append(queue, filepath.Join(osRelative, osChildName))
				}
			}
		}
	}
	return nil
}
