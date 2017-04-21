// +build !windows

package gps

import "testing"

func TestStripVendorSymlinks(t *testing.T) {
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
