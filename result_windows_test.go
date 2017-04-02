package gps

import "testing"

func TestStripVendorJunction(t *testing.T) {
	type testcase struct {
		before, after filesystemState
	}

	t.Run("vendor junction", stripVendorTestCase(fsTestCase{
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

}
