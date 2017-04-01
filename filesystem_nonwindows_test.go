// +build !windows

package gps

import (
	"os"
	"testing"
)

// setup inflates fs onto the actual host file system
func (fs filesystemState) setup(t *testing.T) {
	for _, dir := range fs.dirs {
		p := dir.prepend(fs.root)
		if err := os.MkdirAll(p.String(), 0777); err != nil {
			t.Fatalf("os.MkdirAll(%q, 0777) err=%q", p, err)
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
