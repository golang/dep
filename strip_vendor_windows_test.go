// +build windows

package gps

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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
		from := link.path.prepend(fs.root)
		to := fsPath{link.to}.prepend(fs.root)
		// There is no way to make junctions in the standard library, so we'll just
		// do what the stdlib's os tests do: run mklink.
		//
		// Also, all junctions must point to absolute paths.
		output, err := exec.Command("cmd", "/c", "mklink", "/J", from.String(), to.String()).CombinedOutput()
		if err != nil {
			t.Fatalf("failed to run mklink %v %v: %v %q", from.String(), to.String(), err, output)
		}
	}
}

func TestStripVendorJunctions(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "TestStripVendor")
	if err != nil {
		t.Fatalf("ioutil.TempDir err=%q", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("os.RemoveAll(%q) err=%q", tempDir, err)
		}
	}()

	state := filesystemState{
		root: tempDir,
		dirs: []fsPath{
			fsPath{"package"},
			fsPath{"package", "_vendor"},
		},
		links: []fsLink{
			fsLink{
				path: fsPath{"package", "vendor"},
				to:   "_vendor",
			},
		},
	}

	state.setupUsingJunctions(t)

	if err := filepath.Walk(tempDir, stripVendor); err != nil {
		t.Errorf("filepath.Walk err=%q", err)
	}

	// State should be unchanged: we skip junctions on windows.
	state.assert(t)
}
