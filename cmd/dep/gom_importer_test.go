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

const testGomProjectRoot = "github.com/golang/notexist"

func TestGomConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	h.TempDir("src")
	h.TempDir(filepath.Join("src", testGomProjectRoot))
	h.TempCopy(filepath.Join(testGomProjectRoot, gomfileName), "gom/Gomfile")

	projectRoot := h.Path(testGomProjectRoot)
	sm, err := gps.NewSourceManager(h.Path(cacheDir))
	h.Must(err)
	defer sm.Release()

	// Capture stderr so we can verify output
	verboseOutput := &bytes.Buffer{}
	logger := log.New(verboseOutput, "", 0)

	g := newGomImporter(logger, false, sm) // Disable verbose so that we don't print values that change each test run
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect Gomfile")
	}

	m, l, err := g.Import(projectRoot, testGomProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}

	goldenFile := "gom/expected_import_output.txt"
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

func TestGomConfig_ConvertProject(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGomImporter(discardLogger, true, sm)
	g.goms = []gomPackage{
		{
			name: "github.com/sdboyer/deptest",
			options: map[string]interface{}{
				"commit": "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
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
	if v != "ff2948a2ac8f538c4ecd55962e919d1e13e74baf" {
		t.Fatalf("Expected manifest constraint to be %q got %q", "ff2948a2ac8f538c4ecd55962e919d1e13e74baf", v)
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

	rev := lpv.Underlying()
	if rev != "ff2948a2ac8f538c4ecd55962e919d1e13e74baf" {
		t.Fatalf("Expected locked revision to be 'ff2948a2ac8f538c4ecd55962e919d1e13e74baf', got %s", rev)
	}
}

func TestGomConfig_ConvertProject_EmptyComment(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	h.TempDir("src")

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	g := newGomImporter(discardLogger, true, sm)
	g.goms = []gomPackage{
		{
			name: "github.com/sdboyer/deptest",
			options: map[string]interface{}{
				"branch": "0.8.0",
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
		t.Fatalf("Expected manifest constraint to be %q got %q", "^0.8.0", v)
	}

	p := lock.P[0]
	if p.Ident().ProjectRoot != "github.com/sdboyer/deptest" {
		t.Fatalf("Expected the lock to have a project for 'github.com/sdboyer/deptest' but got '%s'", p.Ident().ProjectRoot)
	}

	rev := p.Version().String()
	if rev != "0.8.0" {
		t.Fatalf("Expected locked revision to be 'ff2948a2ac8f538c4ecd55962e919d1e13e74baf', got %s", rev)
	}
}

func TestGomConfig_ProjectExistsInLock(t *testing.T) {
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

// Compares two slices of gomPackage and checks if they are equal.
func equalGomImports(a, b []gomPackage) bool {

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
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}

	return true
}
