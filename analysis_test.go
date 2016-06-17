package vsolver

import (
	"reflect"
	"testing"
)

// ExternalReach uses an easily separable algorithm, wmToReach(), to turn a
// discovered set of packages and their imports into a proper external reach
// map.
//
// That algorithm is purely symbolic (no filesystem interaction), and thus is
// easy to test. This is that test.
func TestWorkmapToReach(t *testing.T) {
	table := map[string]struct {
		name    string
		workmap map[string]wm
		basedir string
		out     map[string][]string
		err     error
	}{
		"single": {
			workmap: map[string]wm{
				"foo": {
					ex: make(map[string]struct{}),
					in: make(map[string]struct{}),
				},
			},
			out: map[string][]string{
				"foo": {},
			},
		},
		"no external": {
			workmap: map[string]wm{
				"foo": {
					ex: make(map[string]struct{}),
					in: make(map[string]struct{}),
				},
				"foo/bar": {
					ex: make(map[string]struct{}),
					in: make(map[string]struct{}),
				},
			},
			out: map[string][]string{
				"foo":     {},
				"foo/bar": {},
			},
		},
		"no external with subpkg": {
			workmap: map[string]wm{
				"foo": {
					ex: make(map[string]struct{}),
					in: map[string]struct{}{
						"foo/bar": struct{}{},
					},
				},
				"foo/bar": {
					ex: make(map[string]struct{}),
					in: make(map[string]struct{}),
				},
			},
			out: map[string][]string{
				"foo":     {},
				"foo/bar": {},
			},
		},
		"simple base transitive": {
			workmap: map[string]wm{
				"foo": {
					ex: make(map[string]struct{}),
					in: map[string]struct{}{
						"foo/bar": struct{}{},
					},
				},
				"foo/bar": {
					ex: map[string]struct{}{
						"baz": struct{}{},
					},
					in: make(map[string]struct{}),
				},
			},
			out: map[string][]string{
				"foo": {
					"baz",
				},
				"foo/bar": {
					"baz",
				},
			},
		},
	}

	for name, fix := range table {
		out, err := wmToReach(fix.workmap, fix.basedir)

		if fix.out == nil {
			if err == nil {
				t.Errorf("wmToReach(%q): Error expected but not received", name)
			}
			continue
		}

		if err != nil {
			t.Errorf("wmToReach(%q): %v", name, err)
			continue
		}

		if !reflect.DeepEqual(out, fix.out) {
			t.Errorf("wmToReach(%q): Did not get expected reach map:\n\t(GOT): %s\n\t(WNT): %s", name, out, fix.out)
		}
	}
}
