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
		*convertTestCase
		packages []vndrPackage
	}{
		"semver reference": {
			packages: []vndrPackage{{
				importPath: "github.com/sdboyer/deptest",
				reference:  "v0.8.0",
				repository: "https://github.com/sdboyer/deptest.git",
			}},
			convertTestCase: &convertTestCase{
				projectRoot:    gps.ProjectRoot("github.com/sdboyer/deptest"),
				wantSourceRepo: "https://github.com/sdboyer/deptest.git",
				wantConstraint: "^0.8.0",
				wantRevision:   gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf"),
				wantVersion:    "v0.8.0",
				wantLockCount:  1,
			},
		},
		"revision reference": {
			packages: []vndrPackage{{
				importPath: "github.com/sdboyer/deptest",
				reference:  "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			}},
			convertTestCase: &convertTestCase{
				projectRoot:    gps.ProjectRoot("github.com/sdboyer/deptest"),
				wantConstraint: "^1.0.0",
				wantVersion:    "v1.0.0",
				wantRevision:   gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf"),
				wantLockCount:  1,
			},
		},
		"untagged revision reference": {
			packages: []vndrPackage{{
				importPath: "github.com/carolynvs/deptest-subpkg",
				reference:  "6c41d90f78bb1015696a2ad591debfa8971512d5",
			}},
			convertTestCase: &convertTestCase{
				projectRoot:    gps.ProjectRoot("github.com/carolynvs/deptest-subpkg"),
				wantConstraint: "*",
				wantVersion:    "",
				wantRevision:   gps.Revision("6c41d90f78bb1015696a2ad591debfa8971512d5"),
				wantLockCount:  1,
			},
		},
		"missing importPath": {
			packages: []vndrPackage{{
				reference: "v1.0.0",
			}},
			convertTestCase: &convertTestCase{
				wantConvertErr: true,
			},
		},
		"missing reference": {
			packages: []vndrPackage{{
				importPath: "github.com/sdboyer/deptest",
			}},
			convertTestCase: &convertTestCase{
				wantConvertErr: true,
			},
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
			g := newVndrImporter(discardLogger, true, sm)
			g.packages = testCase.packages

			manifest, lock, convertErr := g.convert(testCase.projectRoot)
			err = validateConvertTestCase(testCase.convertTestCase, manifest, lock, convertErr)
			if err != nil {
				t.Fatalf("%#v", err)
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

	wantM := dep.NewManifest()
	c1, _ := gps.NewSemverConstraint("^0.8.1")
	wantM.Constraints["github.com/sdboyer/deptest"] = gps.ProjectProperties{
		Source:     "https://github.com/sdboyer/deptest.git",
		Constraint: c1,
	}
	c2, _ := gps.NewSemverConstraint("^2.0.0")
	wantM.Constraints["github.com/sdboyer/deptestdos"] = gps.ProjectProperties{
		Constraint: c2,
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
				gps.NewVersion("v0.8.1").Pair("3f4c3bea144e112a69bbe5d8d01c1b09a544253f"),
				nil,
			),
			gps.NewLockedProject(
				gps.ProjectIdentifier{
					ProjectRoot: "github.com/sdboyer/deptestdos",
				},
				gps.NewVersion("v2.0.0").Pair("5c607206be5decd28e6263ffffdcee067266015e"),
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
				reference:  "v1.0.0",
			}, nil))

	t.Run("with repo",
		testcase("github.com/golang/notreal v1.0.0 https://github.com/golang/notreal",
			&vndrPackage{
				importPath: "github.com/golang/notreal",
				reference:  "v1.0.0",
				repository: "https://github.com/golang/notreal",
			}, nil))

	t.Run("trailing comment",
		testcase("github.com/golang/notreal v1.0.0 https://github.com/golang/notreal  # cool comment",
			&vndrPackage{
				importPath: "github.com/golang/notreal",
				reference:  "v1.0.0",
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
