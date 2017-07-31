// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

const testGlideProjectRoot = "github.com/golang/notexist"

var (
	discardLogger = log.New(ioutil.Discard, "", 0)
)

func newTestContext(h *test.Helper) *dep.Ctx {
	h.TempDir("src")
	pwd := h.Path(".")
	return &dep.Ctx{
		GOPATH: pwd,
		Out:    discardLogger,
		Err:    discardLogger,
	}
}

func TestGlideConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testGlideProjectRoot))
	h.TempCopy(filepath.Join(testGlideProjectRoot, glideYamlName), "glide/glide.yaml")
	h.TempCopy(filepath.Join(testGlideProjectRoot, glideLockName), "glide/glide.lock")
	projectRoot := h.Path(testGlideProjectRoot)

	// Capture stderr so we can verify output
	verboseOutput := &bytes.Buffer{}
	ctx.Err = log.New(verboseOutput, "", 0)

	g := newGlideImporter(ctx.Err, false, sm) // Disable verbose so that we don't print values that change each test run
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect the glide configuration files")
	}

	m, l, err := g.Import(projectRoot, testGlideProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}

	goldenFile := "glide/golden.txt"
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

func TestGlideConfig_Import_MissingLockFile(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testGlideProjectRoot))
	h.TempCopy(filepath.Join(testGlideProjectRoot, glideYamlName), "glide/glide.yaml")
	projectRoot := h.Path(testGlideProjectRoot)

	g := newGlideImporter(ctx.Err, true, sm)
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("The glide importer should gracefully handle when only glide.yaml is present")
	}

	m, l, err := g.Import(projectRoot, testGlideProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l != nil {
		t.Fatal("Expected the lock to not be generated")
	}
}

func TestGlideConfig_Convert_Project(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pkg := "github.com/sdboyer/deptest"
	repo := "https://github.com/sdboyer/deptest.git"

	g := newGlideImporter(ctx.Err, true, sm)
	g.yaml = glideYaml{
		Imports: []glidePackage{
			{
				Name:       pkg,
				Repository: repo,
				Reference:  "1.0.0",
			},
		},
	}
	g.lock = &glideLock{
		Imports: []glideLockedPackage{
			{
				Name:       pkg,
				Repository: repo,
				Reference:  "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
		},
	}

	manifest, lock, err := g.convert(testGlideProjectRoot)
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

func TestGlideConfig_Convert_TestProject(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pkg := "github.com/sdboyer/deptest"

	g := newGlideImporter(ctx.Err, true, sm)
	g.yaml = glideYaml{
		TestImports: []glidePackage{
			{
				Name:      pkg,
				Reference: "v1.0.0",
			},
		},
	}
	g.lock = &glideLock{
		TestImports: []glideLockedPackage{
			{
				Name:      pkg,
				Reference: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
		},
	}

	manifest, lock, err := g.convert(testGlideProjectRoot)
	if err != nil {
		t.Fatal(err)
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

func TestGlideConfig_Convert_Ignore(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pkg := "github.com/sdboyer/deptest"

	g := newGlideImporter(ctx.Err, true, sm)
	g.yaml = glideYaml{
		Ignores: []string{pkg},
	}

	manifest, _, err := g.convert(testGlideProjectRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Ignored) != 1 {
		t.Fatalf("Expected the manifest to contain 1 ignored project but got %d", len(manifest.Ignored))
	}
	i := manifest.Ignored[0]
	if i != pkg {
		t.Fatalf("Expected the manifest to ignore %s but got '%s'", pkg, i)
	}
}

func TestGlideConfig_Convert_ExcludeDir(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGlideImporter(ctx.Err, true, sm)
	g.yaml = glideYaml{
		ExcludeDirs: []string{"samples"},
	}

	manifest, _, err := g.convert(testGlideProjectRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Ignored) != 1 {
		t.Fatalf("Expected the manifest to contain 1 ignored project but got %d", len(manifest.Ignored))
	}
	i := manifest.Ignored[0]
	if i != "github.com/golang/notexist/samples" {
		t.Fatalf("Expected the manifest to ignore 'github.com/golang/notexist/samples' but got '%s'", i)
	}
}

func TestGlideConfig_Convert_ExcludeDir_IgnoresMismatchedPackageName(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGlideImporter(ctx.Err, true, sm)
	g.yaml = glideYaml{
		Name:        "github.com/golang/mismatched-package-name",
		ExcludeDirs: []string{"samples"},
	}

	manifest, _, err := g.convert(testGlideProjectRoot)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Ignored) != 1 {
		t.Fatalf("Expected the manifest to contain 1 ignored project but got %d", len(manifest.Ignored))
	}
	i := manifest.Ignored[0]
	if i != "github.com/golang/notexist/samples" {
		t.Fatalf("Expected the manifest to ignore 'github.com/golang/notexist/samples' but got '%s'", i)
	}
}

func TestGlideConfig_Convert_WarnsForUnusedFields(t *testing.T) {
	testCases := map[string]glidePackage{
		"specified an os":   {OS: "windows"},
		"specified an arch": {Arch: "i686"},
	}

	for wantWarning, pkg := range testCases {
		t.Run(wantWarning, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()

			pkg.Name = "github.com/sdboyer/deptest"
			pkg.Reference = "v1.0.0"

			ctx := newTestContext(h)
			sm, err := ctx.SourceManager()
			h.Must(err)
			defer sm.Release()

			// Capture stderr so we can verify warnings
			verboseOutput := &bytes.Buffer{}
			ctx.Err = log.New(verboseOutput, "", 0)

			g := newGlideImporter(ctx.Err, true, sm)
			g.yaml = glideYaml{
				Imports: []glidePackage{pkg},
			}

			_, _, err = g.convert(testGlideProjectRoot)
			if err != nil {
				t.Fatal(err)
			}

			warnings := verboseOutput.String()
			if !strings.Contains(warnings, wantWarning) {
				t.Errorf("Expected the output to include the warning '%s'", wantWarning)
			}
		})
	}
}

func TestGlideConfig_Convert_BadInput_EmptyPackageName(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGlideImporter(ctx.Err, true, sm)
	g.yaml = glideYaml{
		Imports: []glidePackage{{Name: ""}},
	}

	_, _, err = g.convert(testGlideProjectRoot)
	if err == nil {
		t.Fatal("Expected conversion to fail because the package name is empty")
	}
}
