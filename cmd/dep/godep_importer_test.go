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

const testProjectRoot = "github.com/golang/notexist"

func TestGodepConfig_Convert(t *testing.T) {
	testCases := map[string]struct {
		*convertTestCase
		json godepJSON
	}{
		"convert project": {
			json: godepJSON{
				Imports: []godepPackage{
					{
						ImportPath: "github.com/sdboyer/deptest",
						// This revision has 2 versions attached to it, v1.0.0 & v0.8.0.
						Rev:     "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
						Comment: "v0.8.0",
					},
				},
			},
			convertTestCase: &convertTestCase{
				projectRoot:    gps.ProjectRoot("github.com/sdboyer/deptest"),
				wantConstraint: "^0.8.0",
				wantRevision:   gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf"),
				wantVersion:    "v0.8.0",
				wantLockCount:  1,
			},
		},
		"with semver suffix": {
			json: godepJSON{
				Imports: []godepPackage{
					{
						ImportPath: "github.com/sdboyer/deptest",
						Rev:        "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
						Comment:    "v1.12.0-12-g2fd980e",
					},
				},
			},
			convertTestCase: &convertTestCase{

				projectRoot:    gps.ProjectRoot("github.com/sdboyer/deptest"),
				wantConstraint: "^1.12.0-12-g2fd980e",
				wantLockCount:  1,
				wantVersion:    "v1.0.0",
			},
		},
		"empty comment": {
			json: godepJSON{
				Imports: []godepPackage{
					{
						ImportPath: "github.com/sdboyer/deptest",
						// This revision has 2 versions attached to it, v1.0.0 & v0.8.0.
						Rev: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			convertTestCase: &convertTestCase{

				projectRoot:    gps.ProjectRoot("github.com/sdboyer/deptest"),
				wantConstraint: "^1.0.0",
				wantRevision:   gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf"),
				wantVersion:    "v1.0.0",
				wantLockCount:  1,
			},
		},
		"bad input - empty package name": {
			json: godepJSON{
				Imports: []godepPackage{{ImportPath: ""}},
			},
			convertTestCase: &convertTestCase{

				wantConvertErr: true,
			},
		},
		"bad input - empty revision": {
			json: godepJSON{
				Imports: []godepPackage{
					{
						ImportPath: "github.com/sdboyer/deptest",
					},
				},
			},
			convertTestCase: &convertTestCase{

				wantConvertErr: true,
			},
		},
		"sub-packages": {
			json: godepJSON{
				Imports: []godepPackage{
					{
						ImportPath: "github.com/sdboyer/deptest",
						// This revision has 2 versions attached to it, v1.0.0 & v0.8.0.
						Rev: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
					{
						ImportPath: "github.com/sdboyer/deptest/foo",
						// This revision has 2 versions attached to it, v1.0.0 & v0.8.0.
						Rev: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			convertTestCase: &convertTestCase{

				projectRoot:    gps.ProjectRoot("github.com/sdboyer/deptest"),
				wantLockCount:  1,
				wantConstraint: "^1.0.0",
				wantVersion:    "v1.0.0",
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
			g := newGodepImporter(discardLogger, true, sm)
			g.json = testCase.json

			manifest, lock, convertErr := g.convert(testCase.projectRoot)
			err := validateConvertTestCase(testCase.convertTestCase, manifest, lock, convertErr)
			if err != nil {
				t.Fatalf("%#v", err)
			}
		})
	}
}

func TestGodepConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	h.TempDir("src")
	h.TempDir(filepath.Join("src", testProjectRoot))
	h.TempCopy(filepath.Join(testProjectRoot, godepPath), "godep/Godeps.json")

	projectRoot := h.Path(testProjectRoot)
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

	m, l, err := g.Import(projectRoot, testProjectRoot)
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
			t.Fatalf("want %s, got %s", want, got)
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

	h.TempCopy(filepath.Join(testProjectRoot, godepPath), "godep/Godeps.json")

	projectRoot := h.Path(testProjectRoot)

	g := newGodepImporter(ctx.Err, true, nil)
	err := g.load(projectRoot)
	if err != nil {
		t.Fatalf("Error while loading... %v", err)
	}

	if !equalImports(g.json.Imports, wantJSON.Imports) {
		t.Fatalf("Expected imports to be equal. \n\t(GOT): %v\n\t(WNT): %v", g.json.Imports, wantJSON.Imports)
	}
}

// equalImports compares two slices of godepPackage and checks if they are
// equal.
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
