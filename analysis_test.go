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
					"empty": PackageOrErr{
						Err: &build.NoGoError{
							Dir: j("empty"),
						},
					},
				},
			},
			err: nil,
		},
		"code only": {
			fileRoot:   j("simple"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
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
		},
		"impose import path": {
			fileRoot:   j("simple"),
			importRoot: "arbitrary",
			out: PackageTree{
				ImportRoot: "arbitrary",
				Packages: map[string]PackageOrErr{
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
		},
		"test only": {
			fileRoot:   j("t"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
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
		},
		"xtest only": {
			fileRoot:   j("xt"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
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
		},
		"code and test": {
			fileRoot:   j("simplet"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
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
		},
		"code and xtest": {
			fileRoot:   j("simplext"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
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
		},
		"code, test, xtest": {
			fileRoot:   j("simpleallt"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
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
		},
		"one pkg multifile": {
			fileRoot:   j("m1p"),
			importRoot: "m1p",
			out: PackageTree{
				ImportRoot: "m1p",
				Packages: map[string]PackageOrErr{
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
		},
		"one nested below": {
			fileRoot:   j("nest"),
			importRoot: "nest",
			out: PackageTree{
				ImportRoot: "nest",
				Packages: map[string]PackageOrErr{
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
		},
		"two nested under empty root": {
			fileRoot:   j("ren"),
			importRoot: "ren",
			out: PackageTree{
				ImportRoot: "ren",
				Packages: map[string]PackageOrErr{
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
		},
		"code and ignored main": {
			fileRoot:   j("igmain"),
			importRoot: "simple",
			out: PackageTree{
				ImportRoot: "simple",
				Packages: map[string]PackageOrErr{
					"simple": PackageOrErr{
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/vsolver",
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
					"simple": PackageOrErr{
						P: Package{
							ImportPath:  "simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/vsolver",
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
		"two pkgs": {
			fileRoot:   j("twopkgs"),
			importRoot: "twopkgs",
			out: PackageTree{
				ImportRoot: "twopkgs",
				Packages: map[string]PackageOrErr{
					"twopkgs": PackageOrErr{
						Err: &build.MultiplePackageError{
							Dir:      j("twopkgs"),
							Packages: []string{"simple", "m1p"},
							Files:    []string{"a.go", "b.go"},
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
					"varied": PackageOrErr{
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
					"varied/otherpath": PackageOrErr{
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
					"varied/simple": PackageOrErr{
						P: Package{
							ImportPath:  "varied/simple",
							CommentPath: "",
							Name:        "simple",
							Imports: []string{
								"github.com/sdboyer/vsolver",
								"go/parser",
								"varied/simple/another",
							},
						},
					},
					"varied/simple/another": PackageOrErr{
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
					"varied/namemismatch": PackageOrErr{
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
					"varied/m1p": PackageOrErr{
						P: Package{
							ImportPath:  "varied/m1p",
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
		},
	}

	for name, fix := range table {
		if _, err := os.Stat(fix.fileRoot); err != nil {
			t.Errorf("listPackages(%q): error on fileRoot %s: %s", name, fix.fileRoot, err)
			continue
		}

		out, err := listPackages(fix.fileRoot, fix.importRoot)

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
						t.Errorf("listPackages(%q): Did not get expected PackageOrErrs:\n\t(GOT): %s\n\t(WNT): %s", name, out, fix.out)
					} else {
						seen := make(map[string]bool)
						for path, perr := range fix.out.Packages {
							seen[path] = true
							if operr, exists := out.Packages[path]; !exists {
								t.Errorf("listPackages(%q): Expected PackageOrErr for path %s was missing from output:\n\t%s", path, perr)
							} else {
								if !reflect.DeepEqual(perr, operr) {
									t.Errorf("listPackages(%q): PkgOrErr for path %s was not as expected:\n\t(GOT): %s\n\t(WNT): %s", name, path, operr, perr)
								}
							}
						}

						for path, operr := range out.Packages {
							if seen[path] {
								continue
							}

							t.Errorf("listPackages(%q): Got PackageOrErr for path %s, but none was expected:\n\t%s", path, operr)
						}
					}
				}
			}
		}
	}
}

func TestListExternalImports(t *testing.T) {
	// There's enough in the 'varied' test case to test most of what matters
	vptree, err := listPackages(filepath.Join(getwd(t), "_testdata", "src", "varied"), "varied")
	if err != nil {
		t.Fatalf("listPackages failed on varied test case: %s", err)
	}

	var expect []string
	var name string
	var ignore map[string]bool
	var main, tests bool

	validate := func() {
		result, err := vptree.ListExternalImports(main, tests, ignore)
		if err != nil {
			t.Errorf("%q case returned err: %s", name, err)
		}
		if !reflect.DeepEqual(expect, result) {
			t.Errorf("Wrong imports in %q case:\n\t(GOT): %s\n\t(WNT): %s", name, result, expect)
		}
	}

	all := []string{
		"encoding/binary",
		"github.com/Masterminds/semver",
		"github.com/sdboyer/vsolver",
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
		"varied": true,
	}
	except("net/http")
	validate()

	// now drop a more interesting one
	name = "ignore simple"
	ignore = map[string]bool{
		"varied/simple": true,
	}
	// we get github.com/sdboyer/vsolver from m1p, too, so it should still be
	// there
	except("go/parser")
	validate()

	// now drop two
	name = "ignore simple and namemismatch"
	ignore = map[string]bool{
		"varied/simple":       true,
		"varied/namemismatch": true,
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

	// ignore two that should knock out vsolver
	name = "ignore both importers"
	ignore = map[string]bool{
		"varied/simple": true,
		"varied/m1p":    true,
	}
	except("sort", "github.com/sdboyer/vsolver", "go/parser")
	validate()

	// finally, directly ignore some external packages
	name = "ignore external"
	ignore = map[string]bool{
		"github.com/sdboyer/vsolver": true,
		"go/parser":                  true,
		"sort":                       true,
	}
	except("sort", "github.com/sdboyer/vsolver", "go/parser")
	validate()
}

func getwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}
