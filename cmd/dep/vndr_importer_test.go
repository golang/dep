// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"log"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

func TestVndrConfig_Convert(t *testing.T) {
	testCases := map[string]struct {
		vndr               []vndrPackage
		wantConvertErr     bool
		matchPairedVersion bool
		projectRoot        gps.ProjectRoot
		wantConstraint     string
		wantRevision       gps.Revision
		wantVersion        string
		wantLockCount      int
	}{
		"project": {
			vndr: []vndrPackage{{
				importPath: "github.com/sdboyer/deptest",
				revision:   "v0.8.0",
				repository: "https://github.com/sdboyer/deptest.git",
			}},
			matchPairedVersion: false,
			projectRoot:        gps.ProjectRoot("github.com/sdboyer/deptest"),
			wantConstraint:     "^0.8.0",
			wantRevision:       gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf"),
			wantVersion:        "v0.8.0",
			wantLockCount:      1,
		},
		"with semver suffix": {
			vndr: []vndrPackage{{
				importPath: "github.com/sdboyer/deptest",
				revision:   "v1.12.0-12-g2fd980e",
			}},
			matchPairedVersion: false,
			projectRoot:        gps.ProjectRoot("github.com/sdboyer/deptest"),
			wantConstraint:     "^1.12.0-12-g2fd980e",
			wantVersion:        "v1.0.0",
			wantLockCount:      1,
		},
		"hash revision": {
			vndr: []vndrPackage{{
				importPath: "github.com/sdboyer/deptest",
				revision:   "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			}},
			matchPairedVersion: false,
			projectRoot:        gps.ProjectRoot("github.com/sdboyer/deptest"),
			wantConstraint:     "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			wantVersion:        "v1.0.0",
			wantLockCount:      1,
		},
		"missing importPath": {
			vndr: []vndrPackage{{
				revision: "v1.0.0",
			}},
			wantConvertErr: true,
		},
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			v := newVndrImporter(discardLogger, true, sm)
			v.packages = testCase.vndr

			manifest, lock, err := v.convert(testCase.projectRoot)
			if err != nil {
				if testCase.wantConvertErr {
					return
				}
				t.Fatal(err)
			} else {
				if testCase.wantConvertErr {
					t.Fatal("expected err, have nil")
				}
			}

			if len(lock.P) != testCase.wantLockCount {
				t.Fatalf("Expected lock to have %d project(s), got %d",
					testCase.wantLockCount,
					len(lock.P))
			}

			d, ok := manifest.Constraints[testCase.projectRoot]
			if !ok {
				t.Fatalf("Expected the manifest to have a dependency for '%s' but got none",
					testCase.projectRoot)
			}

			c := d.Constraint.String()
			if c != testCase.wantConstraint {
				t.Fatalf("Expected manifest constraint to be %s, got %s", testCase.wantConstraint, c)
			}

			p := lock.P[0]
			if p.Ident().ProjectRoot != testCase.projectRoot {
				t.Fatalf("Expected the lock to have a project for '%s' but got '%s'",
					testCase.projectRoot,
					p.Ident().ProjectRoot)
			}

			lv := p.Version()
			lpv, ok := lv.(gps.PairedVersion)

			if !ok {
				if testCase.matchPairedVersion {
					t.Fatalf("Expected locked version to be PairedVersion but got %T", lv)
				}

				return
			}

			ver := lpv.String()
			if ver != testCase.wantVersion {
				t.Fatalf("Expected locked version to be '%s', got %s", testCase.wantVersion, ver)
			}

			if testCase.wantRevision != "" {
				rev := lpv.Revision()
				if rev != testCase.wantRevision {
					t.Fatalf("Expected locked revision to be '%s', got %s",
						testCase.wantRevision,
						rev)
				}
			}
		})
	}

}

func TestVndrConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testProjectRoot))
	h.TempCopy(vndrFile(testProjectRoot), "vndr/vendor.conf")
	projectRoot := h.Path(testProjectRoot)

	logOutput := bytes.NewBuffer(nil)
	ctx.Err = log.New(logOutput, "", 0)

	v := newVndrImporter(ctx.Err, false, sm)
	if !v.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect vndr configuration file")
	}

	m, l, err := v.Import(projectRoot, testProjectRoot)
	h.Must(err)

	constraint, err := gps.NewSemverConstraint("^2.0.0")
	h.Must(err)
	wantM := &dep.Manifest{
		Constraints: gps.ProjectConstraints{
			"github.com/sdboyer/deptest": gps.ProjectProperties{
				Source:     "https://github.com/sdboyer/deptest.git",
				Constraint: gps.Revision("3f4c3bea144e112a69bbe5d8d01c1b09a544253f"),
			},
			"github.com/sdboyer/deptestdos": gps.ProjectProperties{
				Constraint: constraint,
			},
		},
	}
	if !reflect.DeepEqual(wantM, m) {
		t.Errorf("unexpected manifest\nhave=%+v\nwant=%+v", m, wantM)
	}

	wantL := &dep.Lock{
		P: []gps.LockedProject{
			gps.NewLockedProject(
				gps.ProjectIdentifier{
					ProjectRoot: "github.com/sdboyer/deptest",
					Source:      "https://github.com/sdboyer/deptest.git",
				},
				gps.NewVersion("v0.8.1").Pair(gps.Revision("3f4c3bea144e112a69bbe5d8d01c1b09a544253f")),
				nil,
			),
			gps.NewLockedProject(
				gps.ProjectIdentifier{
					ProjectRoot: "github.com/sdboyer/deptestdos",
				},
				gps.Revision("v2.0.0"),
				nil,
			),
		},
	}
	if !reflect.DeepEqual(wantL, l) {
		t.Errorf("unexpected lock\nhave=%+v\nwant=%+v", l, wantL)
	}

	goldenFile := "vndr/golden.txt"
	got := logOutput.String()
	want := h.GetTestFileString(goldenFile)
	if want != got {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenFile, got); err != nil {
				t.Fatalf("%+v", errors.Wrapf(err, "Unable to write updated golden file %s", goldenFile))
			}
		} else {
			t.Fatalf("expected %s, got %s", want, got)
		}
	}
}

func TestParseVndrLine(t *testing.T) {
	testcase := func(in string, wantPkg *vndrPackage, wantErr error) func(*testing.T) {
		return func(t *testing.T) {
			havePkg, haveErr := parseVndrLine(in)
			switch {
			case wantPkg == nil:
				if havePkg != nil {
					t.Errorf("expected nil package, have %v", havePkg)
				}
			case havePkg == nil:
				if wantPkg != nil {
					t.Errorf("expected non-nil package %v, have nil", wantPkg)
				}
			default:
				if !reflect.DeepEqual(havePkg, wantPkg) {
					t.Errorf("unexpected package, have=%v, want=%v", *havePkg, *wantPkg)
				}
			}

			switch {
			case wantErr == nil:
				if haveErr != nil {
					t.Errorf("expected nil err, have %v", haveErr)
				}
			case haveErr == nil:
				if wantErr != nil {
					t.Errorf("expected non-nil err %v, have nil", wantErr)
				}
			default:
				if haveErr.Error() != wantErr.Error() {
					t.Errorf("expected err=%q, have err=%q", wantErr.Error(), haveErr.Error())
				}
			}
		}
	}
	t.Run("normal line",
		testcase("github.com/golang/notreal v1.0.0",
			&vndrPackage{
				importPath: "github.com/golang/notreal",
				revision:   "v1.0.0",
			}, nil))

	t.Run("with repo",
		testcase("github.com/golang/notreal v1.0.0 https://github.com/golang/notreal",
			&vndrPackage{
				importPath: "github.com/golang/notreal",
				revision:   "v1.0.0",
				repository: "https://github.com/golang/notreal",
			}, nil))

	t.Run("trailing comment",
		testcase("github.com/golang/notreal v1.0.0 https://github.com/golang/notreal  # cool comment",
			&vndrPackage{
				importPath: "github.com/golang/notreal",
				revision:   "v1.0.0",
				repository: "https://github.com/golang/notreal",
			}, nil))

	t.Run("empty line", testcase("", nil, nil))
	t.Run("comment line", testcase("# comment", nil, nil))
	t.Run("comment line with leading whitespace", testcase("  # comment", nil, nil))

	t.Run("missing revision",
		testcase("github.com/golang/notreal", nil,
			errors.New("invalid config format: \"github.com/golang/notreal\""),
		))
}
