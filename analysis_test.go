package vsolver

import (
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// externalReach() uses an easily separable algorithm, wmToReach(), to turn a
// discovered set of packages and their imports into a proper external reach
// map.
//
// That algorithm is purely symbolic (no filesystem interaction), and thus is
// easy to test. This is that test.
func TestWorkmapToReach(t *testing.T) {
	empty := func() map[string]struct{} {
		return make(map[string]struct{})
	}

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
					ex: empty(),
					in: empty(),
				},
			},
			out: map[string][]string{
				"foo": {},
			},
		},
		"no external": {
			workmap: map[string]wm{
				"foo": {
					ex: empty(),
					in: empty(),
				},
				"foo/bar": {
					ex: empty(),
					in: empty(),
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
					ex: empty(),
					in: map[string]struct{}{
						"foo/bar": struct{}{},
					},
				},
				"foo/bar": {
					ex: empty(),
					in: empty(),
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
					ex: empty(),
					in: map[string]struct{}{
						"foo/bar": struct{}{},
					},
				},
				"foo/bar": {
					ex: map[string]struct{}{
						"baz": struct{}{},
					},
					in: empty(),
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

func TestListPackages(t *testing.T) {
	srcdir := filepath.Join(getwd(t), "_testdata", "src")
	j := func(s string) string {
		return filepath.Join(srcdir, s)
	}

	table := map[string]struct {
		fileRoot   string // if left empty, will be filled to <cwd>/_testdata/src
		importRoot string
		tests      bool
		out        PackageTree
		err        error
	}{
		"empty": {
			fileRoot:   j("empty"),
			importRoot: "empty",
			tests:      true,
			out:        nil,
			err:        nil,
		},
		"code only": {
			fileRoot:   j("simple"),
			importRoot: "simple",
			tests:      true,
			out: PackageTree{
				"simple": PackageOrErr{
					P: Package{
						ImportPath:  "simple",
						CommentPath: "",
						Name:        "simple",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"sort",
						},
					},
				},
			},
		},
		"impose import path": {
			fileRoot:   j("simple"),
			importRoot: "arbitrary",
			tests:      true,
			out: PackageTree{
				"arbitrary": PackageOrErr{
					P: Package{
						ImportPath:  "arbitrary",
						CommentPath: "",
						Name:        "simple",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"sort",
						},
					},
				},
			},
		},
		"test only": {
			fileRoot:   j("t"),
			importRoot: "simple",
			tests:      true,
			out: PackageTree{
				"simple": PackageOrErr{
					P: Package{
						ImportPath:  "simple",
						CommentPath: "",
						Name:        "simple",
						Imports:     []string{},
						TestImports: []string{
							"math/rand",
							"strconv",
						},
					},
				},
			},
		},
		"xtest only": {
			fileRoot:   j("xt"),
			importRoot: "simple",
			tests:      true,
			out: PackageTree{
				"simple": PackageOrErr{
					P: Package{
						ImportPath:  "simple",
						CommentPath: "",
						Name:        "simple",
						Imports:     []string{},
						TestImports: []string{
							"sort",
							"strconv",
						},
					},
				},
			},
		},
		"code and test": {
			fileRoot:   j("simplet"),
			importRoot: "simple",
			tests:      true,
			out: PackageTree{
				"simple": PackageOrErr{
					P: Package{
						ImportPath:  "simple",
						CommentPath: "",
						Name:        "simple",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"sort",
						},
						TestImports: []string{
							"math/rand",
							"strconv",
						},
					},
				},
			},
		},
		"code and xtest": {
			fileRoot:   j("simplext"),
			importRoot: "simple",
			tests:      true,
			out: PackageTree{
				"simple": PackageOrErr{
					P: Package{
						ImportPath:  "simple",
						CommentPath: "",
						Name:        "simple",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"sort",
						},
						TestImports: []string{
							"sort",
							"strconv",
						},
					},
				},
			},
		},
		"code, test, xtest": {
			fileRoot:   j("simpleallt"),
			importRoot: "simple",
			tests:      true,
			out: PackageTree{
				"simple": PackageOrErr{
					P: Package{
						ImportPath:  "simple",
						CommentPath: "",
						Name:        "simple",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"sort",
						},
						TestImports: []string{
							"math/rand",
							"sort",
							"strconv",
						},
					},
				},
			},
		},
		"one pkg multifile": {
			fileRoot:   j("m1p"),
			importRoot: "m1p",
			tests:      true,
			out: PackageTree{
				"m1p": PackageOrErr{
					P: Package{
						ImportPath:  "m1p",
						CommentPath: "",
						Name:        "m1p",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"os",
							"sort",
						},
					},
				},
			},
		},
		"one nested below": {
			fileRoot:   j("nest"),
			importRoot: "nest",
			tests:      true,
			out: PackageTree{
				"nest": PackageOrErr{
					P: Package{
						ImportPath:  "nest",
						CommentPath: "",
						Name:        "simple",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"sort",
						},
					},
				},
				"nest/m1p": PackageOrErr{
					P: Package{
						ImportPath:  "nest/m1p",
						CommentPath: "",
						Name:        "m1p",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"os",
							"sort",
						},
					},
				},
			},
		},
		"two nested under empty root": {
			fileRoot:   j("ren"),
			importRoot: "ren",
			tests:      true,
			out: PackageTree{
				"ren": PackageOrErr{
					Err: &build.NoGoError{
						Dir: j("ren"),
					},
				},
				"ren/m1p": PackageOrErr{
					P: Package{
						ImportPath:  "ren/m1p",
						CommentPath: "",
						Name:        "m1p",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"os",
							"sort",
						},
					},
				},
				"ren/simple": PackageOrErr{
					P: Package{
						ImportPath:  "ren/simple",
						CommentPath: "",
						Name:        "simple",
						Imports: []string{
							"github.com/sdboyer/vsolver",
							"sort",
						},
					},
				},
			},
		},
	}

	for name, fix := range table {
		if _, err := os.Stat(fix.fileRoot); err != nil {
			t.Errorf("listPackages(%q): error on fileRoot %s: %s", name, fix.fileRoot, err)
			continue
		}

		out, err := llistPackages(fix.fileRoot, fix.importRoot, fix.tests)

		if err != nil && fix.err == nil {
			t.Errorf("listPackages(%q): Received error but none expected: %s", name, err)
		} else if fix.err != nil && err == nil {
			t.Errorf("listPackages(%q): Error expected but none received", name)
		} else if fix.err != nil && err != nil {
			if !reflect.DeepEqual(fix.err, err) {
				t.Errorf("listPackages(%q): Did not receive expected error:\n\t(GOT): %s\n\t(WNT): %s", name, err, fix.err)
			}
		}

		if fix.out != nil {
			if !reflect.DeepEqual(out, fix.out) {
				t.Errorf("listPackages(%q): Did not receive expected package:\n\t(GOT): %s\n\t(WNT): %s", name, out, fix.out)
			}
		}
	}
}

func getwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}
