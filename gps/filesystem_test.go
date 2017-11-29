// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"os"
	"path/filepath"
	"testing"
)

// This file contains utilities for running tests around file system state.

type fsTestCase struct {
	before, after filesystemState
}

// assert makes sure that the tc.after state matches the state of the actual host
// file system at tc.after.root.
func (tc fsTestCase) assert(t *testing.T) {
	dirMap := make(map[string]bool)
	fileMap := make(map[string]bool)
	linkMap := make(map[string]bool)

	for _, d := range tc.after.dirs {
		dirMap[filepath.Join(tc.after.root, d)] = true
	}
	for _, f := range tc.after.files {
		fileMap[filepath.Join(tc.after.root, f)] = true
	}
	for _, l := range tc.after.links {
		linkMap[filepath.Join(tc.after.root, l.path)] = true
	}

	err := filepath.Walk(tc.after.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Errorf("filepath.Walk path=%q  err=%q", path, err)
			return err
		}

		if path == tc.after.root {
			return nil
		}

		// Careful! Have to check whether the path is a symlink first because, on
		// windows, a symlink to a directory will return 'true' for info.IsDir().
		if (info.Mode() & os.ModeSymlink) != 0 {
			if linkMap[path] {
				delete(linkMap, path)
			} else {
				t.Errorf("unexpected symlink exists %q", path)
			}
			return nil
		}

		if info.IsDir() {
			if dirMap[path] {
				delete(dirMap, path)
			} else {
				t.Errorf("unexpected directory exists %q", path)
			}
			return nil
		}

		if fileMap[path] {
			delete(fileMap, path)
		} else {
			t.Errorf("unexpected file exists %q", path)
		}
		return nil
	})

	if err != nil {
		t.Errorf("filesystem.Walk err=%q", err)
	}

	for d := range dirMap {
		t.Errorf("could not find expected directory %q", d)
	}
	for f := range fileMap {
		t.Errorf("could not find expected file %q", f)
	}
	for l := range linkMap {
		t.Errorf("could not find expected symlink %q", l)
	}
}

// setup inflates fs onto the actual host file system at tc.before.root.
// It doesn't delete existing files and should be used on empty roots only.
func (tc fsTestCase) setup(t *testing.T) {
	tc.setupDirs(t)
	tc.setupFiles(t)
	tc.setupLinks(t)
}

func (tc fsTestCase) setupDirs(t *testing.T) {
	for _, dir := range tc.before.dirs {
		p := filepath.Join(tc.before.root, dir)
		if err := os.MkdirAll(p, 0777); err != nil {
			t.Fatalf("os.MkdirAll(%q, 0777) err=%q", p, err)
		}
	}
}

func (tc fsTestCase) setupFiles(t *testing.T) {
	for _, file := range tc.before.files {
		p := filepath.Join(tc.before.root, file)
		f, err := os.Create(p)
		if err != nil {
			t.Fatalf("os.Create(%q) err=%q", p, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("file %q Close() err=%q", p, err)
		}
	}
}

func (tc fsTestCase) setupLinks(t *testing.T) {
	for _, link := range tc.before.links {
		p := filepath.Join(tc.before.root, link.path)

		// On Windows, relative symlinks confuse filepath.Walk. This is golang/go
		// issue 17540. So, we'll just sigh and do absolute links, assuming they are
		// relative to the directory of link.path.
		dir := filepath.Dir(p)
		to := filepath.Join(dir, link.to)

		if err := os.Symlink(to, p); err != nil {
			t.Fatalf("os.Symlink(%q, %q) err=%q", to, p, err)
		}
	}
}
