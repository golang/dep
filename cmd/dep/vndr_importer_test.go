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
