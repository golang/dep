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
	p := fsPath{prefix}
	return append(p, f...)
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

// setup inflates fs onto the actual host file system
func (fs filesystemState) setup(t *testing.T) {
	for _, dir := range fs.dirs {
		p := dir.prepend(fs.root)
		if err := os.MkdirAll(p.String(), 0777); err != nil {
			t.Fatalf("os.MkdirAll(%q, 0777) err=%q", p, 0777)
		}
	}
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
	for _, link := range fs.links {
		p := link.path.prepend(fs.root)
		if err := os.Symlink(link.to, p.String()); err != nil {
			t.Fatalf("os.Symlink(%q, %q) err=%q", link.to, p, err)
		}
	}
}

// assert makes sure that the fs state matches the state of the actual host
// file system
func (fs filesystemState) assert(t *testing.T) {
	dirMap := make(map[string]struct{})
	fileMap := make(map[string]struct{})
	linkMap := make(map[string]struct{})

	for _, d := range fs.dirs {
		dirMap[d.prepend(fs.root).String()] = struct{}{}
	}
	for _, f := range fs.files {
		fileMap[f.prepend(fs.root).String()] = struct{}{}
	}
	for _, l := range fs.links {
		linkMap[l.path.prepend(fs.root).String()] = struct{}{}
	}

	filepath.Walk(fs.root, func(path string, info os.FileInfo, err error) error {
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
			_, ok := linkMap[path]
			if !ok {
				t.Errorf("unexpected symlink exists %q", path)
			} else {
				delete(linkMap, path)
			}
			return nil
		}

		if info.IsDir() {
			_, ok := dirMap[path]
			if !ok {
				t.Errorf("unexpected directory exists %q", path)
			} else {
				delete(dirMap, path)
			}
			return nil
		}

		_, ok := fileMap[path]
		if !ok {
			t.Errorf("unexpected file exists %q", path)
		} else {
			delete(fileMap, path)
		}
		return nil
	})

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
