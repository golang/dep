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
