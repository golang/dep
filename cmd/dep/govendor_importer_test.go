// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

const testGovendorProjectRoot = "github.com/golang/notexist"

func TestGovendorConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testGovendorProjectRoot))
	h.TempCopy(filepath.Join(testGovendorProjectRoot, govendorDir, govendorName), "govendor/vendor.json")
	projectRoot := h.Path(testGovendorProjectRoot)

	// Capture stderr so we can verify output
	verboseOutput := &bytes.Buffer{}
	ctx.Err = log.New(verboseOutput, "", 0)

	g := newGovendorImporter(ctx.Err, false, sm) // Disable verbose so that we don't print values that change each test run
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect the govendor configuration files")
	}

	m, l, err := g.Import(projectRoot, testGovendorProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}

	goldenFile := "govendor/golden.txt"
	got := verboseOutput.String()
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

func TestGovendorConfig_Convert_Project(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pkg := "github.com/sdboyer/deptest"

	g := newGovendorImporter(ctx.Err, true, sm)
	g.file = govendorFile{
		Package: []*govendorPackage{
			{
				Path:     pkg,
				Revision: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				Version:  "v1.0.0",
			},
		},
	}

	manifest, lock, err := g.convert(testGovendorProjectRoot)
	if err != nil {
		t.Fatal(err)
	}

	if manifest == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if lock == nil {
		t.Fatal("Expected the lock file to be generated")
	}

	d, ok := manifest.Constraints[gps.ProjectRoot(pkg)]
	if !ok {
		t.Fatal("Expected the manifest to have a dependency for 'github.com/sdboyer/deptest' but got none")
	}

	wantC := "^1.0.0"
	gotC := d.Constraint.String()
	if gotC != wantC {
		t.Fatalf("Expected manifest constraint to be %s, got %s", wantC, gotC)
	}

	wantP := 1
	gotP := len(lock.P)
	if gotP != 1 {
		t.Fatalf("Expected the lock to contain %d project but got %d", wantP, gotP)
	}

	p := lock.P[0]
	gotPr := string(p.Ident().ProjectRoot)
	if gotPr != pkg {
		t.Fatalf("Expected the lock to have a project for %s but got '%s'", pkg, gotPr)
	}

	lv := p.Version()
	lpv, ok := lv.(gps.PairedVersion)
	if !ok {
		t.Fatalf("Expected locked version to be a PairedVersion but got %T", lv)
	}

	wantRev := "ff2948a2ac8f538c4ecd55962e919d1e13e74baf"
	gotRev := lpv.Revision().String()
	if gotRev != wantRev {
		t.Fatalf("Expected locked revision to be %s, got %s", wantRev, gotRev)
	}

	wantV := "v1.0.0"
	gotV := lpv.String()
	if gotV != wantV {
		t.Fatalf("Expected locked version to be %s, got %s", wantV, gotV)
	}
}

func TestGovendorConfig_Convert_TestProject(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pkg := "github.com/sdboyer/deptest"

	g := newGovendorImporter(ctx.Err, true, sm)
	g.file = govendorFile{
		Package: []*govendorPackage{
			{
				Path:     pkg,
				Revision: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				Version:  "v1.0.0",
			},
		},
	}

	manifest, lock, err := g.convert(testGovendorProjectRoot)
	if err != nil {
		t.Fatal(err)
	}

	if manifest == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if lock == nil {
		t.Fatal("Expected the lock file to be generated")
	}

	_, ok := manifest.Constraints[gps.ProjectRoot(pkg)]
	if !ok {
		t.Fatalf("Expected the manifest to have a dependency for %s but got none", pkg)
	}

	if len(lock.P) != 1 {
		t.Fatalf("Expected the lock to contain 1 project but got %d", len(lock.P))
	}
	p := lock.P[0]
	if p.Ident().ProjectRoot != gps.ProjectRoot(pkg) {
		t.Fatalf("Expected the lock to have a project for %s but got '%s'", pkg, p.Ident().ProjectRoot)
	}
}

func TestGovendorConfig_Convert_Ignore(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pkg := "github.com/sdboyer/deptest"

	g := newGovendorImporter(ctx.Err, true, sm)
	g.file = govendorFile{
		Ignore: strings.Join([]string{"test", pkg, "linux_amd64", "github.com/sdboyer/"}, " "),
	}

	m, _, err := g.convert(testGovendorProjectRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Ignored) != 1 {
		t.Fatalf("Expected the ignored list to contain 1 project but got %d", len(m.Ignored))
	}

	p := m.Ignored[0]
	if p != pkg {
		t.Fatalf("Expected the ignored list to have an element for %s but got '%s'", pkg, p)
	}
}

func TestGovendorConfig_Convert_BadInput_EmptyPackagePath(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGovendorImporter(ctx.Err, true, sm)
	g.file = govendorFile{
		Package: []*govendorPackage{{Path: ""}},
	}

	_, _, err = g.convert(testGovendorProjectRoot)
	if err == nil {
		t.Fatal("Expected conversion to fail because the package name is empty")
	}
}
