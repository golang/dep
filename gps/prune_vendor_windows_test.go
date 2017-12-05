// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build windows

package gps

import "testing"

func TestPruneVendorSymlinks(t *testing.T) {
	// On windows, we skip symlinks, even if they're named 'vendor', because
	// they're too hard to distinguish from junctions.
	t.Run("vendor symlink", pruneVendorDirsTestCase(fsTestCase{
		before: filesystemState{
			dirs: []string{
				"package",
				"package/_vendor",
			},
			links: []fsLink{
				{
					path: "package/vendor",
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []string{
				"package",
				"package/_vendor",
			},
			links: []fsLink{
				{
					path: "package/vendor",
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("nonvendor symlink", pruneVendorDirsTestCase(fsTestCase{
		before: filesystemState{
			dirs: []string{
				"package",
				"package/_vendor",
			},
			links: []fsLink{
				{
					path: "package/link",
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []string{
				"package",
				"package/_vendor",
			},
			links: []fsLink{
				{
					path: "package/link",
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("vendor symlink to file", pruneVendorDirsTestCase(fsTestCase{
		before: filesystemState{
			files: []string{
				"file",
			},
			links: []fsLink{
				{
					path: "vendor",
					to:   "file",
				},
			},
		},
		after: filesystemState{
			files: []string{
				"file",
			},
			links: []fsLink{
				{
					path: "vendor",
					to:   "file",
				},
			},
		},
	}))

	t.Run("broken vendor symlink", pruneVendorDirsTestCase(fsTestCase{
		before: filesystemState{
			dirs: []string{
				"package",
			},
			links: []fsLink{
				{
					path: "package/vendor",
					to:   "nonexistence",
				},
			},
		},
		after: filesystemState{
			dirs: []string{
				"package",
			},
			links: []fsLink{
				{
					path: "package/vendor",
					to:   "nonexistence",
				},
			},
		},
	}))

	t.Run("chained symlinks", pruneVendorDirsTestCase(fsTestCase{
		// Curiously, if a symlink on windows points to *another* symlink which
		// eventually points at a directory, we'll correctly remove that first
		// symlink, because the first symlink doesn't appear to Go to be a
		// directory.
		before: filesystemState{
			dirs: []string{
				"_vendor",
			},
			links: []fsLink{
				{
					path: "vendor",
					to:   "vendor2",
				},
				{
					path: "vendor2",
					to:   "_vendor",
				},
			},
		},
		after: filesystemState{
			dirs: []string{
				"_vendor",
			},
			links: []fsLink{
				{
					path: "vendor2",
					to:   "_vendor",
				},
			},
		},
	}))

	t.Run("circular symlinks", pruneVendorDirsTestCase(fsTestCase{
		before: filesystemState{
			dirs: []string{
				"package",
			},
			links: []fsLink{
				{
					path: "package/link1",
					to:   "link2",
				},
				{
					path: "package/link2",
					to:   "link1",
				},
			},
		},
		after: filesystemState{
			dirs: []string{
				"package",
			},
			links: []fsLink{
				{
					path: "package/link1",
					to:   "link2",
				},
				{
					path: "package/link2",
					to:   "link1",
				},
			},
		},
	}))
}
