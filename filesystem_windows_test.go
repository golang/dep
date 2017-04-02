// +build windows

package gps

import (
	"os/exec"
	"testing"
)

// setupUsingJunctions inflates fs onto the host file system, but uses Windows
// directory junctions for links.
func (fs filesystemState) setupUsingJunctions(t *testing.T) {
	fs.setupDirs(t)
	fs.setupFiles(t)
	fs.setupJunctions(t)
}

func (fs filesystemState) setupJunctions(t *testing.T) {
	for _, link := range fs.links {
		p := link.path.prepend(fs.root)
		// There is no way to make junctions in the standard library, so we'll just
		// do what the stdlib's os tests do: run mklink.
		output, err := exec.Command("cmd", "/c", "mklink", "/J", p.String(), link.to).CombinedOutput()
		if err != nil {
			t.Fatalf("failed to run mklink %v %v: %v %q", p.String(), link.to, err, output)
		}
	}
}
