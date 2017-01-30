package gps

import (
	"fmt"
	"go/build"
	"go/scanner"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// PackageTree.ExternalReach() uses an easily separable algorithm, wmToReach(),
// to turn a discovered set of packages and their imports into a proper external
// reach map.
//
// That algorithm is purely symbolic (no filesystem interaction), and thus is
// easy to test. This is that test.
func TestWorkmapToReach(t *testing.T) {
	empty := func() map[string]bool {
		return make(map[string]bool)
	}

	table := map[string]struct {
		workmap map[string]wm
		out     ReachMap
	}{
		"single": {
			workmap: map[string]wm{
				"foo": {
					ex: empty(),
					in: empty(),
				},
			},
			out: ReachMap{
				"foo": nil,
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
			out: ReachMap{
				"foo":     nil,
				"foo/bar": nil,
			},
		},
		"no external with subpkg": {
			workmap: map[string]wm{
				"foo": {
					ex: empty(),
					in: map[string]bool{
						"foo/bar": true,
					},
				},
				"foo/bar": {
					ex: empty(),
					in: empty(),
				},
			},
			out: ReachMap{
				"foo":     nil,
				"foo/bar": nil,
			},
		},
		"simple base transitive": {
			workmap: map[string]wm{
				"foo": {
					ex: empty(),
					in: map[string]bool{
						"foo/bar": true,
					},
				},
				"foo/bar": {
					ex: map[string]bool{
						"baz": true,
					},
					in: empty(),
				},
			},
			out: ReachMap{
				"foo": {
					"baz",
				},
				"foo/bar": {
					"baz",
				},
			},
		},
		"missing package is poison": {
			workmap: map[string]wm{
				"A": {
					ex: map[string]bool{
						"B/foo": true,
					},
					in: map[string]bool{
						"A/foo": true, // missing
						"A/bar": true,
					},
				},
				"A/bar": {
					ex: map[string]bool{
						"B/baz": true,
					},
					in: empty(),
				},
			},
			out: ReachMap{
				"A/bar": {
					"B/baz",
				},
			},
		},
		"transitive missing package is poison": {
			workmap: map[string]wm{
				"A": {
					ex: map[string]bool{
						"B/foo": true,
					},
					in: map[string]bool{
						"A/foo":  true, // transitively missing
						"A/quux": true,
					},
				},
				"A/foo": {
					ex: map[string]bool{
						"C/flugle": true,
					},
					in: map[string]bool{
						"A/bar": true, // missing
					},
				},
				"A/quux": {
					ex: map[string]bool{
						"B/baz": true,
					},
					in: empty(),
				},
			},
			out: ReachMap{
				"A/quux": {
					"B/baz",
				},
			},
		},
		"err'd package is poison": {
			workmap: map[string]wm{
				"A": {
					ex: map[string]bool{
						"B/foo": true,
					},
					in: map[string]bool{
						"A/foo": true, // err'd
						"A/bar": true,
					},
				},
				"A/foo": {
					err: fmt.Errorf("err pkg"),
				},
				"A/bar": {
					ex: map[string]bool{
						"B/baz": true,
					},
					in: empty(),
				},
			},
			out: ReachMap{
				"A/bar": {
					"B/baz",
				},
			},
		},
		"transitive err'd package is poison": {
			workmap: map[string]wm{
				"A": {
					ex: map[string]bool{
						"B/foo": true,
					},
					in: map[string]bool{
						"A/foo":  true, // transitively err'd
						"A/quux": true,
					},
				},
				"A/foo": {
					ex: map[string]bool{
						"C/flugle": true,
					},
					in: map[string]bool{
						"A/bar": true, // err'd
					},
				},
				"A/bar": {
					err: fmt.Errorf("err pkg"),
				},
				"A/quux": {
					ex: map[string]bool{
						"B/baz": true,
					},
					in: empty(),
				},
			},
			out: ReachMap{
				"A/quux": {
					"B/baz",
				},
			},
		},
	}

	for name, fix := range table {
		out, _ := wmToReach(fix.workmap)
		if !reflect.DeepEqual(out, fix.out) {
			t.Errorf("wmToReach(%q): Did not get expected reach map:\n\t(GOT): %#v\n\t(WNT): %#v", name, out, fix.out)
		}
	}
}

func TestListPackagesNoDir(t *testing.T) {
	out, err := ListPackages(filepath.Join(getwd(t), "_testdata", "notexist"), "notexist")
	if err == nil {
		t.Error("ListPackages should have errored on pointing to a nonexistent dir")
	}
	if !reflect.DeepEqual(PackageTree{}, out) {
		t.Error("should've gotten back an empty PackageTree")
	}
}

func TestListPackages(t *testing.T) {
	srcdir := filepath.Join(getwd(t), "_testdata", "src")
	j := func(s ...string) string {
		return filepath.Join(srcdir, filepath.Join(s...))
	}

	table := map[string]struct {
		fileRoot   string
		importRoot string
		out        PackageTree
		err        error
	}{
		"empty": {
			fileRoot:   j("empty"),
			importRoot: "empty",
			out: PackageTree{
				ImportRoot: "empty",
				Packages: map[string]PackageOrErr{
					"empty": {
						Err: &build.NoGoError{
							Dir: j("empty"),
						},
					},
				},
			},
		},
		"code only": {
			fileRoot:   j("simple"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
							},
						},
					},
				},
			},
		},
		"impose import path": {
			fileRoot:   j("simple"),
			importRoot: "arbitrary",
			out: PackageTree{
				ImportRoot: "arbitrary",
				Packages: map[string]PackageOrErr{
					"arbitrary": {
						P: Package{
							ImportPath:  "arbitrary",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
							},
						},
					},
				},
			},
		},
		"test only": {
			fileRoot:   j("t"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
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
		},
		"xtest only": {
			fileRoot:   j("xt"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
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
		},
		"code and test": {
			fileRoot:   j("simplet"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
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
		},
		"code and xtest": {
			fileRoot:   j("simplext"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
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
		},
		"code, test, xtest": {
			fileRoot:   j("simpleallt"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
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
		},
		"one pkg multifile": {
			fileRoot:   j("m1p"),
			importRoot: "m1p",
			out: PackageTree{
				ImportRoot: "m1p",
				Packages: map[string]PackageOrErr{
					"m1p": {
						P: Package{
							ImportPath:  "m1p",
							CommentPath: "",
							Name:        "m1p",
							Imports: []string{
								"github.com/sdboyer/gps",
								"os",
								"sort",
							},
						},
					},
				},
			},
		},
		"one nested below": {
			fileRoot:   j("nest"),
			importRoot: "nest",
			out: PackageTree{
				ImportRoot: "nest",
				Packages: map[string]PackageOrErr{
					"nest": {
						P: Package{
							ImportPath:  "nest",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
							},
						},
					},
					"nest/m1p": {
						P: Package{
							ImportPath:  "nest/m1p",
							CommentPath: "",
							Name:        "m1p",
							Imports: []string{
								"github.com/sdboyer/gps",
								"os",
								"sort",
							},
						},
					},
				},
			},
		},
		"malformed go file": {
			fileRoot:   j("bad"),
			importRoot: "bad",
			out: PackageTree{
				ImportRoot: "bad",
				Packages: map[string]PackageOrErr{
					"bad": {
						Err: scanner.ErrorList{
							&scanner.Error{
								Pos: token.Position{
									Filename: j("bad", "bad.go"),
									Offset:   113,
									Line:     2,
									Column:   43,
								},
								Msg: "expected 'package', found 'EOF'",
							},
						},
					},
				},
			},
		},
		"two nested under empty root": {
			fileRoot:   j("ren"),
			importRoot: "ren",
			out: PackageTree{
				ImportRoot: "ren",
				Packages: map[string]PackageOrErr{
					"ren": {
						Err: &build.NoGoError{
							Dir: j("ren"),
						},
					},
					"ren/m1p": {
						P: Package{
							ImportPath:  "ren/m1p",
							CommentPath: "",
							Name:        "m1p",
							Imports: []string{
								"github.com/sdboyer/gps",
								"os",
								"sort",
							},
						},
					},
					"ren/simple": {
						P: Package{
							ImportPath:  "ren/simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
							},
						},
					},
				},
			},
		},
		"internal name mismatch": {
			fileRoot:   j("doublenest"),
			importRoot: "doublenest",
			out: PackageTree{
				ImportRoot: "doublenest",
				Packages: map[string]PackageOrErr{
					"doublenest": {
						P: Package{
							ImportPath:  "doublenest",
							CommentPath: "",
							Name:        "base",
							Imports: []string{
								"github.com/sdboyer/gps",
								"go/parser",
							},
						},
					},
					"doublenest/namemismatch": {
						P: Package{
							ImportPath:  "doublenest/namemismatch",
							CommentPath: "",
							Name:        "nm",
							Imports: []string{
								"github.com/Masterminds/semver",
								"os",
							},
						},
					},
					"doublenest/namemismatch/m1p": {
						P: Package{
							ImportPath:  "doublenest/namemismatch/m1p",
							CommentPath: "",
							Name:        "m1p",
							Imports: []string{
								"github.com/sdboyer/gps",
								"os",
								"sort",
							},
						},
					},
				},
			},
		},
		"file and importroot mismatch": {
			fileRoot:   j("doublenest"),
			importRoot: "other",
			out: PackageTree{
				ImportRoot: "other",
				Packages: map[string]PackageOrErr{
					"other": {
						P: Package{
							ImportPath:  "other",
							CommentPath: "",
							Name:        "base",
							Imports: []string{
								"github.com/sdboyer/gps",
								"go/parser",
							},
						},
					},
					"other/namemismatch": {
						P: Package{
							ImportPath:  "other/namemismatch",
							CommentPath: "",
							Name:        "nm",
							Imports: []string{
								"github.com/Masterminds/semver",
								"os",
							},
						},
					},
					"other/namemismatch/m1p": {
						P: Package{
							ImportPath:  "other/namemismatch/m1p",
							CommentPath: "",
							Name:        "m1p",
							Imports: []string{
								"github.com/sdboyer/gps",
								"os",
								"sort",
							},
						},
					},
				},
			},
		},
		"code and ignored main": {
			fileRoot:   j("igmain"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
								"unicode",
							},
						},
					},
				},
			},
		},
		"code and ignored main, order check": {
			fileRoot:   j("igmainfirst"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
								"unicode",
							},
						},
					},
				},
			},
		},
		"code and ignored main with comment leader": {
			fileRoot:   j("igmainlong"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
								"unicode",
							},
						},
					},
				},
			},
		},
		"code, tests, and ignored main": {
			fileRoot:   j("igmaint"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": {
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"sort",
								"unicode",
							},
							TestImports: []string{
								"math/rand",
								"strconv",
							},
						},
					},
				},
			},
		},
		// New code allows this because it doesn't care if the code compiles (kinda) or not,
		// so maybe this is actually not an error anymore?
		//
		// TODO re-enable this case after the full and proper ListPackages()
		// refactor in #99
		/*"two pkgs": {
			fileRoot:   j("twopkgs"),
			importRoot: "twopkgs",
			out: PackageTree{
				ImportRoot: "twopkgs",
				Packages: map[string]PackageOrErr{
					"twopkgs": {
						Err: &build.MultiplePackageError{
							Dir:      j("twopkgs"),
							Packages: []string{"simple", "m1p"},
							Files:    []string{"a.go", "b.go"},
						},
					},
				},
			},
		}, */
		// imports a missing pkg
		"missing import": {
			fileRoot:   j("missing"),
			importRoot: "missing",
			out: PackageTree{
				ImportRoot: "missing",
				Packages: map[string]PackageOrErr{
					"missing": {
						P: Package{
							ImportPath:  "missing",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"missing/missing",
								"sort",
							},
						},
					},
					"missing/m1p": {
						P: Package{
							ImportPath:  "missing/m1p",
							CommentPath: "",
							Name:        "m1p",
							Imports: []string{
								"github.com/sdboyer/gps",
								"os",
								"sort",
							},
						},
					},
				},
			},
		},
		// import cycle of three packages. ListPackages doesn't do anything
		// special with cycles - that's the reach calculator's job - so this is
		// error-free
		"import cycle, len 3": {
			fileRoot:   j("cycle"),
			importRoot: "cycle",
			out: PackageTree{
				ImportRoot: "cycle",
				Packages: map[string]PackageOrErr{
					"cycle": {
						P: Package{
							ImportPath:  "cycle",
							CommentPath: "",
							Name:        "cycle",
							Imports: []string{
								"cycle/one",
								"github.com/sdboyer/gps",
							},
						},
					},
					"cycle/one": {
						P: Package{
							ImportPath:  "cycle/one",
							CommentPath: "",
							Name:        "one",
							Imports: []string{
								"cycle/two",
								"github.com/sdboyer/gps",
							},
						},
					},
					"cycle/two": {
						P: Package{
							ImportPath:  "cycle/two",
							CommentPath: "",
							Name:        "two",
							Imports: []string{
								"cycle",
								"github.com/sdboyer/gps",
							},
						},
					},
				},
			},
		},
		// has disallowed dir names
		"disallowed dirs": {
			fileRoot:   j("disallow"),
			importRoot: "disallow",
			out: PackageTree{
				ImportRoot: "disallow",
				Packages: map[string]PackageOrErr{
					"disallow": {
						P: Package{
							ImportPath:  "disallow",
							CommentPath: "",
							Name:        "disallow",
							Imports: []string{
								"disallow/testdata",
								"github.com/sdboyer/gps",
								"sort",
							},
						},
					},
					// disallow/.m1p is ignored by listPackages...for now. Kept
					// here commented because this might change again...
					//"disallow/.m1p": {
					//P: Package{
					//ImportPath:  "disallow/.m1p",
					//CommentPath: "",
					//Name:        "m1p",
					//Imports: []string{
					//"github.com/sdboyer/gps",
					//"os",
					//"sort",
					//},
					//},
					//},
					"disallow/testdata": {
						P: Package{
							ImportPath:  "disallow/testdata",
							CommentPath: "",
							Name:        "testdata",
							Imports: []string{
								"hash",
							},
						},
					},
				},
			},
		},
		"relative imports": {
			fileRoot:   j("relimport"),
			importRoot: "relimport",
			out: PackageTree{
				ImportRoot: "relimport",
				Packages: map[string]PackageOrErr{
					"relimport": {
						P: Package{
							ImportPath:  "relimport",
							CommentPath: "",
							Name:        "relimport",
							Imports: []string{
								"sort",
							},
						},
					},
					"relimport/dot": {
						P: Package{
							ImportPath:  "relimport/dot",
							CommentPath: "",
							Name:        "dot",
							Imports: []string{
								".",
								"sort",
							},
						},
					},
					"relimport/dotdot": {
						Err: &LocalImportsError{
							Dir:        j("relimport/dotdot"),
							ImportPath: "relimport/dotdot",
							LocalImports: []string{
								"..",
							},
						},
					},
					"relimport/dotslash": {
						Err: &LocalImportsError{
							Dir:        j("relimport/dotslash"),
							ImportPath: "relimport/dotslash",
							LocalImports: []string{
								"./simple",
							},
						},
					},
					"relimport/dotdotslash": {
						Err: &LocalImportsError{
							Dir:        j("relimport/dotdotslash"),
							ImportPath: "relimport/dotdotslash",
							LocalImports: []string{
								"../github.com/sdboyer/gps",
							},
						},
					},
				},
			},
		},
		// This case mostly exists for the PackageTree methods, but it does
		// cover a bit of range
		"varied": {
			fileRoot:   j("varied"),
			importRoot: "varied",
			out: PackageTree{
				ImportRoot: "varied",
				Packages: map[string]PackageOrErr{
					"varied": {
						P: Package{
							ImportPath:  "varied",
							CommentPath: "",
							Name:        "main",
							Imports: []string{
								"net/http",
								"varied/namemismatch",
								"varied/otherpath",
								"varied/simple",
							},
						},
					},
					"varied/otherpath": {
						P: Package{
							ImportPath:  "varied/otherpath",
							CommentPath: "",
							Name:        "otherpath",
							Imports:     []string{},
							TestImports: []string{
								"varied/m1p",
							},
						},
					},
					"varied/simple": {
						P: Package{
							ImportPath:  "varied/simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/gps",
								"go/parser",
								"varied/simple/another",
							},
						},
					},
					"varied/simple/another": {
						P: Package{
							ImportPath:  "varied/simple/another",
							CommentPath: "",
							Name:        "another",
							Imports: []string{
								"hash",
								"varied/m1p",
							},
							TestImports: []string{
								"encoding/binary",
							},
						},
					},
					"varied/namemismatch": {
						P: Package{
							ImportPath:  "varied/namemismatch",
							CommentPath: "",
							Name:        "nm",
							Imports: []string{
								"github.com/Masterminds/semver",
								"os",
							},
						},
					},
					"varied/m1p": {
						P: Package{
							ImportPath:  "varied/m1p",
							CommentPath: "",
							Name:        "m1p",
							Imports: []string{
								"github.com/sdboyer/gps",
								"os",
								"sort",
							},
						},
					},
				},
			},
		},
		"invalid buildtag like comments should be ignored": {
			fileRoot:   j("buildtag"),
			importRoot: "buildtag",
			out: PackageTree{
				ImportRoot: "buildtag",
				Packages: map[string]PackageOrErr{
					"buildtag": {
						P: Package{
							ImportPath:  "buildtag",
							CommentPath: "",
							Name:        "buildtag",
							Imports: []string{
								"sort",
							},
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

		out, err := ListPackages(fix.fileRoot, fix.importRoot)

		if err != nil && fix.err == nil {
			t.Errorf("listPackages(%q): Received error but none expected: %s", name, err)
		} else if fix.err != nil && err == nil {
			t.Errorf("listPackages(%q): Error expected but none received", name)
		} else if fix.err != nil && err != nil {
			if !reflect.DeepEqual(fix.err, err) {
				t.Errorf("listPackages(%q): Did not receive expected error:\n\t(GOT): %s\n\t(WNT): %s", name, err, fix.err)
			}
		}

		if fix.out.ImportRoot != "" && fix.out.Packages != nil {
			if !reflect.DeepEqual(out, fix.out) {
				if fix.out.ImportRoot != out.ImportRoot {
					t.Errorf("listPackages(%q): Expected ImportRoot %s, got %s", name, fix.out.ImportRoot, out.ImportRoot)
				}

				// overwrite the out one to see if we still have a real problem
				out.ImportRoot = fix.out.ImportRoot

				if !reflect.DeepEqual(out, fix.out) {
					if len(fix.out.Packages) < 2 {
						t.Errorf("listPackages(%q): Did not get expected PackageOrErrs:\n\t(GOT): %#v\n\t(WNT): %#v", name, out, fix.out)
					} else {
						seen := make(map[string]bool)
						for path, perr := range fix.out.Packages {
							seen[path] = true
							if operr, exists := out.Packages[path]; !exists {
								t.Errorf("listPackages(%q): Expected PackageOrErr for path %s was missing from output:\n\t%s", name, path, perr)
							} else {
								if !reflect.DeepEqual(perr, operr) {
									t.Errorf("listPackages(%q): PkgOrErr for path %s was not as expected:\n\t(GOT): %#v\n\t(WNT): %#v", name, path, operr, perr)
								}
							}
						}

						for path, operr := range out.Packages {
							if seen[path] {
								continue
							}

							t.Errorf("listPackages(%q): Got PackageOrErr for path %s, but none was expected:\n\t%s", name, path, operr)
						}
					}
				}
			}
		}
	}
}

// Test that ListPackages skips directories for which it lacks permissions to
// enter and files it lacks permissions to read.
func TestListPackagesNoPerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		// TODO This test doesn't work on windows because I wasn't able to easily
		// figure out how to chmod a dir in a way that made it untraversable.
		//
		// It's not a big deal, though, because the os.IsPermission() call we
		// use in the real code is effectively what's being tested here, and
		// that's designed to be cross-platform. So, if the unix tests pass, we
		// have every reason to believe windows tests would to, if the situation
		// arises.
		t.Skip()
	}
	tmp, err := ioutil.TempDir("", "listpkgsnp")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
		t.FailNow()
	}
	defer os.RemoveAll(tmp)

	srcdir := filepath.Join(getwd(t), "_testdata", "src", "ren")
	workdir := filepath.Join(tmp, "ren")
	copyDir(srcdir, workdir)

	// chmod the simple dir and m1p/b.go file so they can't be read
	err = os.Chmod(filepath.Join(workdir, "simple"), 0)
	if err != nil {
		t.Error("Error while chmodding simple dir", err)
		t.FailNow()
	}
	os.Chmod(filepath.Join(workdir, "m1p", "b.go"), 0)
	if err != nil {
		t.Error("Error while chmodding b.go file", err)
		t.FailNow()
	}

	want := PackageTree{
		ImportRoot: "ren",
		Packages: map[string]PackageOrErr{
			"ren": {
				Err: &build.NoGoError{
					Dir: workdir,
				},
			},
			"ren/m1p": {
				P: Package{
					ImportPath:  "ren/m1p",
					CommentPath: "",
					Name:        "m1p",
					Imports: []string{
						"github.com/sdboyer/gps",
						"sort",
					},
				},
			},
		},
	}

	got, err := ListPackages(workdir, "ren")

	if err != nil {
		t.Errorf("Unexpected err from ListPackages: %s", err)
		t.FailNow()
	}
	if want.ImportRoot != got.ImportRoot {
		t.Errorf("Expected ImportRoot %s, got %s", want.ImportRoot, got.ImportRoot)
		t.FailNow()
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Did not get expected PackageOrErrs:\n\t(GOT): %#v\n\t(WNT): %#v", got, want)
		if len(got.Packages) != 2 {
			if len(got.Packages) == 3 {
				t.Error("Wrong number of PackageOrErrs - did 'simple' subpackage make it into results somehow?")
			} else {
				t.Error("Wrong number of PackageOrErrs")
			}
		}

		if got.Packages["ren"].Err == nil {
			t.Error("Should have gotten error on empty root directory")
		}

		if !reflect.DeepEqual(got.Packages["ren/m1p"].P.Imports, want.Packages["ren/m1p"].P.Imports) {
			t.Error("Mismatch between imports in m1p")
		}
	}
}

func TestListExternalImports(t *testing.T) {
	// There's enough in the 'varied' test case to test most of what matters
	vptree, err := ListPackages(filepath.Join(getwd(t), "_testdata", "src", "github.com", "example", "varied"), "github.com/example/varied")
	if err != nil {
		t.Fatalf("listPackages failed on varied test case: %s", err)
	}

	var expect []string
	var name string
	var ignore map[string]bool
	var main, tests bool

	validate := func() {
		exmap, _ := vptree.ExternalReach(main, tests, ignore)
		result := exmap.ListExternalImports()
		if !reflect.DeepEqual(expect, result) {
			t.Errorf("Wrong imports in %q case:\n\t(GOT): %s\n\t(WNT): %s", name, result, expect)
		}
	}

	all := []string{
		"encoding/binary",
		"github.com/Masterminds/semver",
		"github.com/sdboyer/gps",
		"go/parser",
		"hash",
		"net/http",
		"os",
		"sort",
	}

	// helper to rewrite expect, except for a couple packages
	//
	// this makes it easier to see what we're taking out on each test
	except := func(not ...string) {
		expect = make([]string, len(all)-len(not))

		drop := make(map[string]bool)
		for _, npath := range not {
			drop[npath] = true
		}

		k := 0
		for _, path := range all {
			if !drop[path] {
				expect[k] = path
				k++
			}
		}
	}

	// everything on
	name = "simple"
	except()
	main, tests = true, true
	validate()

	// Now without tests, which should just cut one
	name = "no tests"
	tests = false
	except("encoding/binary")
	validate()

	// Now skip main, which still just cuts out one
	name = "no main"
	main, tests = false, true
	except("net/http")
	validate()

	// No test and no main, which should be additive
	name = "no test, no main"
	main, tests = false, false
	except("net/http", "encoding/binary")
	validate()

	// now, the ignore tests. turn main and tests back on
	main, tests = true, true

	// start with non-matching
	name = "non-matching ignore"
	ignore = map[string]bool{
		"nomatch": true,
	}
	except()
	validate()

	// should have the same effect as ignoring main
	name = "ignore the root"
	ignore = map[string]bool{
		"github.com/example/varied": true,
	}
	except("net/http")
	validate()

	// now drop a more interesting one
	name = "ignore simple"
	ignore = map[string]bool{
		"github.com/example/varied/simple": true,
	}
	// we get github.com/sdboyer/gps from m1p, too, so it should still be there
	except("go/parser")
	validate()

	// now drop two
	name = "ignore simple and namemismatch"
	ignore = map[string]bool{
		"github.com/example/varied/simple":       true,
		"github.com/example/varied/namemismatch": true,
	}
	except("go/parser", "github.com/Masterminds/semver")
	validate()

	// make sure tests and main play nice with ignore
	name = "ignore simple and namemismatch, and no tests"
	tests = false
	except("go/parser", "github.com/Masterminds/semver", "encoding/binary")
	validate()
	name = "ignore simple and namemismatch, and no main"
	main, tests = false, true
	except("go/parser", "github.com/Masterminds/semver", "net/http")
	validate()
	name = "ignore simple and namemismatch, and no main or tests"
	main, tests = false, false
	except("go/parser", "github.com/Masterminds/semver", "net/http", "encoding/binary")
	validate()

	main, tests = true, true

	// ignore two that should knock out gps
	name = "ignore both importers"
	ignore = map[string]bool{
		"github.com/example/varied/simple": true,
		"github.com/example/varied/m1p":    true,
	}
	except("sort", "github.com/sdboyer/gps", "go/parser")
	validate()

	// finally, directly ignore some external packages
	name = "ignore external"
	ignore = map[string]bool{
		"github.com/sdboyer/gps": true,
		"go/parser":              true,
		"sort":                   true,
	}
	except("sort", "github.com/sdboyer/gps", "go/parser")
	validate()

	// The only thing varied *doesn't* cover is disallowed path patterns
	ptree, err := ListPackages(filepath.Join(getwd(t), "_testdata", "src", "disallow"), "disallow")
	if err != nil {
		t.Fatalf("listPackages failed on disallow test case: %s", err)
	}

	exmap, _ := ptree.ExternalReach(false, false, nil)
	result := exmap.ListExternalImports()
	expect = []string{"github.com/sdboyer/gps", "hash", "sort"}
	if !reflect.DeepEqual(expect, result) {
		t.Errorf("Wrong imports in %q case:\n\t(GOT): %s\n\t(WNT): %s", name, result, expect)
	}
}

func TestExternalReach(t *testing.T) {
	// There's enough in the 'varied' test case to test most of what matters
	vptree, err := ListPackages(filepath.Join(getwd(t), "_testdata", "src", "github.com", "example", "varied"), "github.com/example/varied")
	if err != nil {
		t.Fatalf("ListPackages failed on varied test case: %s", err)
	}

	// Set up vars for validate closure
	var expect map[string][]string
	var name string
	var main, tests bool
	var ignore map[string]bool

	validate := func() {
		result, _ := vptree.ExternalReach(main, tests, ignore)
		if !reflect.DeepEqual(expect, result) {
			seen := make(map[string]bool)
			for ip, epkgs := range expect {
				seen[ip] = true
				if pkgs, exists := result[ip]; !exists {
					t.Errorf("ver(%q): expected import path %s was not present in result", name, ip)
				} else {
					if !reflect.DeepEqual(pkgs, epkgs) {
						t.Errorf("ver(%q): did not get expected package set for import path %s:\n\t(GOT): %s\n\t(WNT): %s", name, ip, pkgs, epkgs)
					}
				}
			}

			for ip, pkgs := range result {
				if seen[ip] {
					continue
				}
				t.Errorf("ver(%q): Got packages for import path %s, but none were expected:\n\t%s", name, ip, pkgs)
			}
		}
	}

	all := map[string][]string{
		"github.com/example/varied":                {"encoding/binary", "github.com/Masterminds/semver", "github.com/sdboyer/gps", "go/parser", "hash", "net/http", "os", "sort"},
		"github.com/example/varied/m1p":            {"github.com/sdboyer/gps", "os", "sort"},
		"github.com/example/varied/namemismatch":   {"github.com/Masterminds/semver", "os"},
		"github.com/example/varied/otherpath":      {"github.com/sdboyer/gps", "os", "sort"},
		"github.com/example/varied/simple":         {"encoding/binary", "github.com/sdboyer/gps", "go/parser", "hash", "os", "sort"},
		"github.com/example/varied/simple/another": {"encoding/binary", "github.com/sdboyer/gps", "hash", "os", "sort"},
	}
	// build a map to validate the exception inputs. do this because shit is
	// hard enough to keep track of that it's preferable not to have silent
	// success if a typo creeps in and we're trying to except an import that
	// isn't in a pkg in the first place
	valid := make(map[string]map[string]bool)
	for ip, expkgs := range all {
		m := make(map[string]bool)
		for _, pkg := range expkgs {
			m[pkg] = true
		}
		valid[ip] = m
	}

	// helper to compose expect, excepting specific packages
	//
	// this makes it easier to see what we're taking out on each test
	except := func(pkgig ...string) {
		// reinit expect with everything from all
		expect = make(map[string][]string)
		for ip, expkgs := range all {
			sl := make([]string, len(expkgs))
			copy(sl, expkgs)
			expect[ip] = sl
		}

		// now build the dropmap
		drop := make(map[string]map[string]bool)
		for _, igstr := range pkgig {
			// split on space; first elem is import path to pkg, the rest are
			// the imports to drop.
			not := strings.Split(igstr, " ")
			var ip string
			ip, not = not[0], not[1:]
			if _, exists := valid[ip]; !exists {
				t.Fatalf("%s is not a package name we're working with, doofus", ip)
			}

			// if only a single elem was passed, though, drop the whole thing
			if len(not) == 0 {
				delete(expect, ip)
				continue
			}

			m := make(map[string]bool)
			for _, imp := range not {
				if !valid[ip][imp] {
					t.Fatalf("%s is not a reachable import of %s, even in the all case", imp, ip)
				}
				m[imp] = true
			}

			drop[ip] = m
		}

		for ip, pkgs := range expect {
			var npkgs []string
			for _, imp := range pkgs {
				if !drop[ip][imp] {
					npkgs = append(npkgs, imp)
				}
			}

			expect[ip] = npkgs
		}
	}

	// first, validate all
	name = "all"
	main, tests = true, true
	except()
	validate()

	// turn off main pkgs, which necessarily doesn't affect anything else
	name = "no main"
	main = false
	except("github.com/example/varied")
	validate()

	// ignoring the "varied" pkg has same effect as disabling main pkgs
	name = "ignore root"
	ignore = map[string]bool{
		"github.com/example/varied": true,
	}
	main = true
	validate()

	// when we drop tests, varied/otherpath loses its link to varied/m1p and
	// varied/simple/another loses its test import, which has a fairly big
	// cascade
	name = "no tests"
	tests = false
	ignore = nil
	except(
		"github.com/example/varied encoding/binary",
		"github.com/example/varied/simple encoding/binary",
		"github.com/example/varied/simple/another encoding/binary",
		"github.com/example/varied/otherpath github.com/sdboyer/gps os sort",
	)

	// almost the same as previous, but varied just goes away completely
	name = "no main or tests"
	main = false
	except(
		"github.com/example/varied",
		"github.com/example/varied/simple encoding/binary",
		"github.com/example/varied/simple/another encoding/binary",
		"github.com/example/varied/otherpath github.com/sdboyer/gps os sort",
	)
	validate()

	// focus on ignores now, so reset main and tests
	main, tests = true, true

	// now, the fun stuff. punch a hole in the middle by cutting out
	// varied/simple
	name = "ignore varied/simple"
	ignore = map[string]bool{
		"github.com/example/varied/simple": true,
	}
	except(
		// root pkg loses on everything in varied/simple/another
		"github.com/example/varied hash encoding/binary go/parser",
		"github.com/example/varied/simple",
	)
	validate()

	// widen the hole by excluding otherpath
	name = "ignore varied/{otherpath,simple}"
	ignore = map[string]bool{
		"github.com/example/varied/otherpath": true,
		"github.com/example/varied/simple":    true,
	}
	except(
		// root pkg loses on everything in varied/simple/another and varied/m1p
		"github.com/example/varied hash encoding/binary go/parser github.com/sdboyer/gps sort",
		"github.com/example/varied/otherpath",
		"github.com/example/varied/simple",
	)
	validate()

	// remove namemismatch, though we're mostly beating a dead horse now
	name = "ignore varied/{otherpath,simple,namemismatch}"
	ignore["github.com/example/varied/namemismatch"] = true
	except(
		// root pkg loses on everything in varied/simple/another and varied/m1p
		"github.com/example/varied hash encoding/binary go/parser github.com/sdboyer/gps sort os github.com/Masterminds/semver",
		"github.com/example/varied/otherpath",
		"github.com/example/varied/simple",
		"github.com/example/varied/namemismatch",
	)
	validate()
}

// Verify that we handle import cycles correctly - drop em all
func TestExternalReachCycle(t *testing.T) {
	ptree, err := ListPackages(filepath.Join(getwd(t), "_testdata", "src", "cycle"), "cycle")
	if err != nil {
		t.Fatalf("ListPackages failed on cycle test case: %s", err)
	}

	rm, _ := ptree.ExternalReach(true, true, nil)

	// FIXME TEMPORARILY COMMENTED UNTIL WE CREATE A BETTER LISTPACKAGES MODEL -
	//if len(rm) > 0 {
	//t.Errorf("should be empty reachmap when all packages are in a cycle, got %v", rm)
	//}

	if len(rm) == 0 {
		t.Error("TEMPORARY: should ignore import cycles, but cycle was eliminated")
	}
}

func getwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}
