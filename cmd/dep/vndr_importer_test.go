// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"log"
	"path/filepath"
	"testing"

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

	//	m, l, err := v.Import(projectRoot, testProjectRoot)
	_, _, err = v.Import(projectRoot, testProjectRoot)
	h.Must(err)

	// if m == nil {
	// 	t.Error("expected manifest to be generated")
	// } else {
	// 	if len(m.Constraints) != 2 {
	// 		t.Errorf("expected 2 constraints in manifest, have %+v", m.Constraints)
	// 	} else {
	// 		p1, ok := m.Constraints[gps.ProjectRoot("github.com/sdboyer/deptest")]
	// 		if !ok {
	// 			t.Errorf("expected constraint to for github.com/sdboyer/deptest, have %+v", m.Constraints)
	// 		} else {
	// 			if want := "https://github.com/sdboyer/deptest"; p1.Source != want {
	// 				t.Errorf("unexpected source, have=%v, want=%v", p1.Source, want)
	// 			}
	// 			if want := "3f4c3bea144e112a69bbe5d8d01c1b09a544253f", p1.Constraint.String() != want {
	// 				t.Errorf("unexpected constraint, have=%v, want=%v", p1.Constraint.String(), want)
	// 			}
	// 		}
	// 	}
	// }

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
