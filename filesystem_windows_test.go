// +build windows

package gps

import (
	"os"
	"path/filepath"
	"testing"
)

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
