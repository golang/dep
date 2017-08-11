// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"log"
	"path/filepath"
	"testing"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

const testGbProjectRoot = "github.com/golang/notexist"

func TestGbConfig_ImportNoVendor(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testGbProjectRoot, "vendor"))
	h.TempCopy(filepath.Join(testGbProjectRoot, "vendor", "_not-a-manifest"), "gb/manifest")
	projectRoot := h.Path(testGbProjectRoot)

	// Capture stderr so we can verify output
	verboseOutput := &bytes.Buffer{}
	ctx.Err = log.New(verboseOutput, "", 0)

	g := newGbImporter(ctx.Err, false, sm) // Disable verbose so that we don't print values that change each test run
	if g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to return false if there's no vendor manifest")
	}
}

func TestGbConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testGbProjectRoot, "vendor"))
	h.TempCopy(filepath.Join(testGbProjectRoot, "vendor", "manifest"), "gb/manifest")
	projectRoot := h.Path(testGbProjectRoot)

	// Capture stderr so we can verify output
	verboseOutput := &bytes.Buffer{}
	ctx.Err = log.New(verboseOutput, "", 0)

	g := newGbImporter(ctx.Err, false, sm) // Disable verbose so that we don't print values that change each test run
	if g.Name() != "gb" {
		t.Fatal("Expected the importer to return the name 'gb'")
	}
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect the gb manifest file")
	}

	m, l, err := g.Import(projectRoot, testGbProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}

	goldenFile := "gb/golden.txt"
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

func TestGbConfig_Convert_Project(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pkg := "github.com/sdboyer/deptest"
	repo := "https://github.com/sdboyer/deptest.git"

	g := newGbImporter(ctx.Err, true, sm)
	g.manifest = gbManifest{
		Dependencies: []gbDependency{
			{
				Importpath: pkg,
				Repository: repo,
				Revision:   "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
		},
	}

	manifest, lock, err := g.convert(testGbProjectRoot)
	if err != nil {
		t.Fatal(err)
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

	gotS := d.Source
	if gotS != repo {
		t.Fatalf("Expected manifest source to be %s, got %s", repo, gotS)
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

	gotS = p.Ident().Source
	if gotS != repo {
		t.Fatalf("Expected locked source to be %s, got '%s'", repo, gotS)
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

func TestGbConfig_Convert_BadInput(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGbImporter(ctx.Err, true, sm)
	g.manifest = gbManifest{
		Dependencies: []gbDependency{{Importpath: ""}},
	}

	_, _, err = g.convert(testGbProjectRoot)
	if err == nil {
		t.Fatal("Expected conversion to fail because the package name is empty")
	}

	g = newGbImporter(ctx.Err, true, sm)
	g.manifest = gbManifest{
		Dependencies: []gbDependency{{Importpath: "github.com/sdboyer/deptest"}},
	}

	_, _, err = g.convert(testGbProjectRoot)
	if err == nil {
		t.Fatal("Expected conversion to fail because the package has no revision")
	}
}
