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

func TestGovendConfig_Convert(t *testing.T) {
	testCases := map[string]struct {
		*convertTestCase
		yaml govendYAML
	}{
		"project": {
			yaml: govendYAML{
				Imports: []govendPackage{
					{
						Path:     "github.com/sdboyer/deptest",
						Revision: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			convertTestCase: &convertTestCase{
				projectRoot:    gps.ProjectRoot("github.com/sdboyer/deptest"),
				wantConstraint: "^1.0.0",
				wantLockCount:  1,
				wantRevision:   gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf"),
				wantVersion:    "v1.0.0",
			},
		},
		"bad input - empty package name": {
			yaml: govendYAML{
				Imports: []govendPackage{
					{
						Path: "",
					},
				},
			},
			convertTestCase: &convertTestCase{
				wantConvertErr: true,
			},
		},

		"bad input - empty revision": {
			yaml: govendYAML{
				Imports: []govendPackage{
					{
						Path: "github.com/sdboyer/deptest",
					},
				},
			},
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
			g := newGovendImporter(discardLogger, true, sm)
			g.yaml = testCase.yaml

			manifest, lock, convertErr := g.convert(testCase.projectRoot)
			err = validateConvertTestCase(testCase.convertTestCase, manifest, lock, convertErr)
			if err != nil {
				t.Fatalf("%#v", err)
			}
		})
	}
}

func TestGovendConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	h.TempDir("src")
	h.TempDir(filepath.Join("src", testProjectRoot))
	h.TempCopy(filepath.Join(testProjectRoot, govendYAMLName), "govend/vendor.yml")

	projectRoot := h.Path(testProjectRoot)
	sm, err := gps.NewSourceManager(h.Path(cacheDir))
	h.Must(err)
	defer sm.Release()

	// Capture stderr so we can verify the import output
	verboseOutput := &bytes.Buffer{}
	logger := log.New(verboseOutput, "", 0)

	// Disable verbose so that we don't print values that change each test run
	g := newGovendImporter(logger, false, sm)
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect govend configuration file")
	}

	m, l, err := g.Import(projectRoot, testProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}

	govendImportOutputFile := "govend/expected_govend_import_output.txt"
	got := verboseOutput.String()
	want := h.GetTestFileString(govendImportOutputFile)
	if want != got {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(govendImportOutputFile, got); err != nil {
				t.Fatalf("%+v", errors.Wrapf(err, "Unable to write updated golden file %s", govendImportOutputFile))
			}
		} else {
			t.Fatalf("want %s, got %s", want, got)
		}
	}

}

func TestGovendConfig_YAMLLoad(t *testing.T) {
	// This is same as cmd/testdata/govend/vendor.yml
	wantYAML := govendYAML{
		Imports: []govendPackage{
			{
				Path:     "github.com/sdboyer/deptest",
				Revision: "3f4c3bea144e112a69bbe5d8d01c1b09a544253f",
			},
			{
				Path:     "github.com/sdboyer/deptestdos",
				Revision: "5c607206be5decd28e6263ffffdcee067266015e",
			},
		},
	}
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	h.TempCopy(filepath.Join(testProjectRoot, govendYAMLName), "govend/vendor.yml")

	projectRoot := h.Path(testProjectRoot)

	g := newGovendImporter(ctx.Err, true, nil)
	err := g.load(projectRoot)
	if err != nil {
		t.Fatalf("Error while loading %v", err)
	}

	if !equalGovendImports(g.yaml.Imports, wantYAML.Imports) {
		t.Fatalf("Expected import to be equal. \n\t(GOT): %v\n\t(WNT): %v", g.yaml.Imports, wantYAML.Imports)
	}
}

func equalGovendImports(a, b []govendPackage) bool {
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
