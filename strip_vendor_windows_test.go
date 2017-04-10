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
		// Junctions, when created, forbid listing of their contents. We need to
		// manually permit that so we can call filepath.Walk.
		output, err = exec.Command("cmd", "icacls", from.String(), "/grant", ":r", "Everyone:F").CombinedOutput()
		if err != nil {
			t.Fatalf("failed to run icacls %v /e /p Everyone:F: %v %q", from.String(), err, output)
		}
	}
}

func TestStripVendorJunction(t *testing.T) {
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

func TestStripVendorSymlinks(t *testing.T) {
	// On windows, we skip symlinks, even if they're named 'vendor', because
	// they're too hard to distinguish from junctions.
	t.Run("vendor symlink", stripVendorTestCase(fsTestCase{
		before: filesystemState{
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
		},
		after: filesystemState{
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
		},
	}))

	t.Run("nonvendor symlink", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
				fsPath{"package", "_vendor"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"package", "link"},
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
				fsPath{"package", "_vendor"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"package", "link"},
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("vendor symlink to file", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			files: []fsPath{
				fsPath{"file"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"vendor"},
					to:   "file",
				},
			},
		},
		after: filesystemState{
			files: []fsPath{
				fsPath{"file"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"vendor"},
					to:   "file",
				},
			},
		},
	}))

	t.Run("chained symlinks", stripVendorTestCase(fsTestCase{
		// Curiously, if a symlink on windows points to *another* symlink which
		// eventually points at a directory, we'll correctly remove that first
		// symlink, because the first symlink doesn't appear to Go to be a
		// directory.
		before: filesystemState{
			dirs: []fsPath{
				fsPath{"_vendor"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"vendor"},
					to:   "vendor2",
				},
				fsLink{
					path: fsPath{"vendor2"},
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				fsPath{"_vendor"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"vendor2"},
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("circular symlinks", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"package", "link1"},
					to:   "link2",
				},
				fsLink{
					path: fsPath{"package", "link2"},
					to:   "link1",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				fsPath{"package"},
			},
			links: []fsLink{
				fsLink{
					path: fsPath{"package", "link1"},
					to:   "link2",
				},
				fsLink{
					path: fsPath{"package", "link2"},
					to:   "link1",
				},
			},
		},
	}))
}
