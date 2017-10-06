// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build windows

package gps

import "testing"

func TestStripVendorSymlinks(t *testing.T) {
	// On windows, we skip symlinks, even if they're named 'vendor', because
	// they're too hard to distinguish from junctions.
	t.Run("vendor symlink", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				{"package"},
				{"package", "_vendor"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "vendor"},
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				{"package"},
				{"package", "_vendor"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "vendor"},
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("nonvendor symlink", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				{"package"},
				{"package", "_vendor"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "link"},
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				{"package"},
				{"package", "_vendor"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "link"},
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("vendor symlink to file", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			files: []fsPath{
				{"file"},
			},
			links: []fsLink{
				{
					path: fsPath{"vendor"},
					to:   "file",
				},
			},
		},
		after: filesystemState{
			files: []fsPath{
				{"file"},
			},
			links: []fsLink{
				{
					path: fsPath{"vendor"},
					to:   "file",
				},
			},
		},
	}))

	t.Run("broken vendor symlink", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				{"package"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "vendor"},
					to:   "nonexistence",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				{"package"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "vendor"},
					to:   "nonexistence",
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
				{"_vendor"},
			},
			links: []fsLink{
				{
					path: fsPath{"vendor"},
					to:   "vendor2",
				},
				{
					path: fsPath{"vendor2"},
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				{"_vendor"},
			},
			links: []fsLink{
				{
					path: fsPath{"vendor2"},
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("circular symlinks", stripVendorTestCase(fsTestCase{
		before: filesystemState{
			dirs: []fsPath{
				{"package"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "link1"},
					to:   "link2",
				},
				{
					path: fsPath{"package", "link2"},
					to:   "link1",
				},
			},
		},
		after: filesystemState{
			dirs: []fsPath{
				{"package"},
			},
			links: []fsLink{
				{
					path: fsPath{"package", "link1"},
					to:   "link2",
				},
				{
					path: fsPath{"package", "link2"},
					to:   "link1",
				},
			},
		},
	}))
}
