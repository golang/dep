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

// fspath represents a file system path in an OS-agnostic way.
type fsPath []string

func (f fsPath) String() string { return filepath.Join(f...) }

func (f fsPath) prepend(prefix string) fsPath {
	p := fsPath{filepath.FromSlash(prefix)}
	return append(p, f...)
}

type fsTestCase struct {
	before, after filesystemState
}

// filesystemState represents the state of a file system. It has a setup method
// which inflates its state to the actual host file system, and an assert
// method which checks that the actual file system matches the described state.
type filesystemState struct {
	root  string
	dirs  []fsPath
	files []fsPath
	links []fsLink
}

// assert makes sure that the fs state matches the state of the actual host
// file system
func (fs filesystemState) assert(t *testing.T) {
	dirMap := make(map[string]bool)
	fileMap := make(map[string]bool)
	linkMap := make(map[string]bool)

	for _, d := range fs.dirs {
		dirMap[d.prepend(fs.root).String()] = true
	}
	for _, f := range fs.files {
		fileMap[f.prepend(fs.root).String()] = true
	}
	for _, l := range fs.links {
		linkMap[l.path.prepend(fs.root).String()] = true
	}

	err := filepath.Walk(fs.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Errorf("filepath.Walk path=%q  err=%q", path, err)
			return err
		}

		if path == fs.root {
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

// fsLink represents a symbolic link.
type fsLink struct {
	path fsPath
	to   string
}

// setup inflates fs onto the actual host file system
func (fs filesystemState) setup(t *testing.T) {
	fs.setupDirs(t)
	fs.setupFiles(t)
	fs.setupLinks(t)
}

func (fs filesystemState) setupDirs(t *testing.T) {
	for _, dir := range fs.dirs {
		p := dir.prepend(fs.root)
		if err := os.MkdirAll(p.String(), 0777); err != nil {
			t.Fatalf("os.MkdirAll(%q, 0777) err=%q", p, err)
		}
	}
}

func (fs filesystemState) setupFiles(t *testing.T) {
	for _, file := range fs.files {
		p := file.prepend(fs.root)
		f, err := os.Create(p.String())
		if err != nil {
			t.Fatalf("os.Create(%q) err=%q", p, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("file %q Close() err=%q", p, err)
		}
	}
}

func (fs filesystemState) setupLinks(t *testing.T) {
	for _, link := range fs.links {
		p := link.path.prepend(fs.root)

		// On Windows, relative symlinks confuse filepath.Walk. This is golang/go
		// issue 17540. So, we'll just sigh and do absolute links, assuming they are
		// relative to the directory of link.path.
		dir := filepath.Dir(p.String())
		to := filepath.Join(dir, link.to)

		if err := os.Symlink(to, p.String()); err != nil {
			t.Fatalf("os.Symlink(%q, %q) err=%q", to, p, err)
		}
	}
}
