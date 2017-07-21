// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"log"
	"path/filepath"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

const testGodepProjectRoot = "github.com/golang/notexist"

func TestGodepConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	h.TempDir("src")
	h.TempDir(filepath.Join("src", testGodepProjectRoot))
	h.TempCopy(filepath.Join(testGodepProjectRoot, "Godeps", godepJSONName), "godep/Godeps.json")

	projectRoot := h.Path(testGodepProjectRoot)
	sm, err := gps.NewSourceManager(h.Path(cacheDir))
	h.Must(err)
	defer sm.Release()

	// Capture stderr so we can verify output
	verboseOutput := &bytes.Buffer{}
	logger := log.New(verboseOutput, "", 0)

	g := newGodepImporter(logger, false, sm) // Disable verbose so that we don't print values that change each test run
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect godep configuration file")
	}

	m, l, err := g.Import(projectRoot, testGodepProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}

	goldenFile := "godep/expected_import_output.txt"
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

func TestGodepConfig_JsonLoad(t *testing.T) {
	// This is same as cmd/dep/testdata/Godeps.json
	wantJSON := godepJSON{
		Imports: []godepPackage{
			{
				ImportPath: "github.com/sdboyer/deptest",
				Rev:        "3f4c3bea144e112a69bbe5d8d01c1b09a544253f",
			},
			{
				ImportPath: "github.com/sdboyer/deptestdos",
				Rev:        "5c607206be5decd28e6263ffffdcee067266015e",
				Comment:    "v2.0.0",
			},
		},
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)

	h.TempCopy(filepath.Join(testGodepProjectRoot, "Godeps", godepJSONName), "godep/Godeps.json")

	projectRoot := h.Path(testGodepProjectRoot)

	g := newGodepImporter(ctx.Err, true, nil)
	err := g.load(projectRoot)
	if err != nil {
		t.Fatalf("Error while loading... %v", err)
	}

	if !equalImports(g.json.Imports, wantJSON.Imports) {
		t.Fatalf("Expected imports to be equal. \n\t(GOT): %v\n\t(WNT): %v", g.json.Imports, wantJSON.Imports)
	}
}

func TestGodepConfig_ConvertProject(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGodepImporter(discardLogger, true, sm)
	g.json = godepJSON{
		Imports: []godepPackage{
			{
				ImportPath: "github.com/sdboyer/deptest",
				// This revision has 2 versions attached to it, v1.0.0 & v0.8.0.
				Rev:     "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				Comment: "v0.8.0",
			},
		},
	}

	manifest, lock, err := g.convert("")
	if err != nil {
		t.Fatal(err)
	}

	d, ok := manifest.Constraints["github.com/sdboyer/deptest"]
	if !ok {
		t.Fatal("Expected the manifest to have a dependency for 'github.com/sdboyer/deptest' but got none")
	}

	v := d.Constraint.String()
	if v != "^0.8.0" {
		t.Fatalf("Expected manifest constraint to be ^0.8.0, got %s", v)
	}

	p := lock.P[0]
	if p.Ident().ProjectRoot != "github.com/sdboyer/deptest" {
		t.Fatalf("Expected the lock to have a project for 'github.com/sdboyer/deptest' but got '%s'", p.Ident().ProjectRoot)
	}

	lv := p.Version()
	lpv, ok := lv.(gps.PairedVersion)
	if !ok {
		t.Fatalf("Expected locked version to be PairedVersion but got %T", lv)
	}

	rev := lpv.Revision()
	if rev != "ff2948a2ac8f538c4ecd55962e919d1e13e74baf" {
		t.Fatalf("Expected locked revision to be 'ff2948a2ac8f538c4ecd55962e919d1e13e74baf', got %s", rev)
	}

	ver := lpv.String()
	if ver != "v0.8.0" {
		t.Fatalf("Expected locked version to be 'v0.8.0', got %s", ver)
	}
}

func TestGodepConfig_ConvertProject_WithSemverSuffix(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGodepImporter(discardLogger, true, sm)
	g.json = godepJSON{
		Imports: []godepPackage{
			{
				ImportPath: "github.com/sdboyer/deptest",
				Rev:        "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				Comment:    "v1.12.0-12-g2fd980e",
			},
		},
	}

	manifest, lock, err := g.convert("")
	if err != nil {
		t.Fatal(err)
	}

	d, ok := manifest.Constraints["github.com/sdboyer/deptest"]
	if !ok {
		t.Fatal("Expected the manifest to have a dependency for 'github.com/sdboyer/deptest' but got none")
	}

	v := d.Constraint.String()
	if v != ">=1.12.0, <=12.0.0-g2fd980e" {
		t.Fatalf("Expected manifest constraint to be >=1.12.0, <=12.0.0-g2fd980e, got %s", v)
	}

	p := lock.P[0]
	if p.Ident().ProjectRoot != "github.com/sdboyer/deptest" {
		t.Fatalf("Expected the lock to have a project for 'github.com/sdboyer/deptest' but got '%s'", p.Ident().ProjectRoot)
	}
}

func TestGodepConfig_ConvertProject_EmptyComment(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	h.TempDir("src")

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGodepImporter(discardLogger, true, sm)
	g.json = godepJSON{
		Imports: []godepPackage{
			{
				ImportPath: "github.com/sdboyer/deptest",
				// This revision has 2 versions attached to it, v1.0.0 & v0.8.0.
				Rev: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
		},
	}

	manifest, lock, err := g.convert("")
	if err != nil {
		t.Fatal(err)
	}

	d, ok := manifest.Constraints["github.com/sdboyer/deptest"]
	if !ok {
		t.Fatal("Expected the manifest to have a dependency for 'github.com/sdboyer/deptest' but got none")
	}

	v := d.Constraint.String()
	if v != "^1.0.0" {
		t.Fatalf("Expected manifest constraint to be ^1.0.0, got %s", v)
	}

	p := lock.P[0]
	if p.Ident().ProjectRoot != "github.com/sdboyer/deptest" {
		t.Fatalf("Expected the lock to have a project for 'github.com/sdboyer/deptest' but got '%s'", p.Ident().ProjectRoot)
	}

	lv := p.Version()
	lpv, ok := lv.(gps.PairedVersion)
	if !ok {
		t.Fatalf("Expected locked version to be PairedVersion but got %T", lv)
	}

	rev := lpv.Revision()
	if rev != "ff2948a2ac8f538c4ecd55962e919d1e13e74baf" {
		t.Fatalf("Expected locked revision to be 'ff2948a2ac8f538c4ecd55962e919d1e13e74baf', got %s", rev)
	}

	ver := lpv.String()
	if ver != "v1.0.0" {
		t.Fatalf("Expected locked version to be 'v1.0.0', got %s", ver)
	}
}

func TestGodepConfig_Convert_BadInput_EmptyPackageName(t *testing.T) {
	g := newGodepImporter(discardLogger, true, nil)
	g.json = godepJSON{
		Imports: []godepPackage{{ImportPath: ""}},
	}

	_, _, err := g.convert("")
	if err == nil {
		t.Fatal("Expected conversion to fail because the ImportPath is empty")
	}
}

func TestGodepConfig_Convert_BadInput_EmptyRev(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	h.TempDir("src")

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGodepImporter(discardLogger, true, sm)
	g.json = godepJSON{
		Imports: []godepPackage{
			{
				ImportPath: "github.com/sdboyer/deptest",
			},
		},
	}

	_, _, err = g.convert("")
	if err == nil {
		t.Fatal("Expected conversion to fail because the Rev is empty")
	}
}

func TestGodepConfig_Convert_SubPackages(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	h.TempDir("src")

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGodepImporter(discardLogger, true, sm)
	g.json = godepJSON{
		Imports: []godepPackage{
			{
				ImportPath: "github.com/sdboyer/deptest",
				Rev:        "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
			{
				ImportPath: "github.com/sdboyer/deptest/foo",
				Rev:        "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
		},
	}

	manifest, lock, err := g.convert("")
	if err != nil {
		t.Fatal(err)
	}

	if _, present := manifest.Constraints["github.com/sdboyer/deptest"]; !present {
		t.Fatal("Expected the manifest to have a dependency for 'github.com/sdboyer/deptest'")
	}

	if _, present := manifest.Constraints["github.com/sdboyer/deptest/foo"]; present {
		t.Fatal("Expected the manifest to not have a dependency for 'github.com/sdboyer/deptest/foo'")
	}

	if len(lock.P) != 1 {
		t.Fatalf("Expected lock to have 1 project, got %v", len(lock.P))
	}

	if string(lock.P[0].Ident().ProjectRoot) != "github.com/sdboyer/deptest" {
		t.Fatal("Expected lock to have 'github.com/sdboyer/deptest'")
	}
}

func TestGodepConfig_ProjectExistsInLock(t *testing.T) {
	lock := &dep.Lock{}
	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	ver := gps.NewVersion("v1.0.0")
	lock.P = append(lock.P, gps.NewLockedProject(pi, ver, nil))

	cases := []struct {
		importPath string
		want       bool
	}{
		{
			importPath: "github.com/sdboyer/deptest",
			want:       true,
		},
		{
			importPath: "github.com/golang/notexist",
			want:       false,
		},
	}

	for _, c := range cases {
		result := projectExistsInLock(lock, c.importPath)

		if result != c.want {
			t.Fatalf("projectExistsInLock result is not as expected: \n\t(GOT) %v \n\t(WNT) %v", result, c.want)
		}
	}
}

// Compares two slices of godepPackage and checks if they are equal.
func equalImports(a, b []godepPackage) bool {

	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
