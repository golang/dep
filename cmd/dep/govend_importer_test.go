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

func TestGovendConfig_Convert(t *testing.T) {
	testCases := map[string]struct {
		yaml govendYAML
		convertTestCase
	}{
		"package": {
			govendYAML{
				Imports: []govendPackage{
					{
						Path:     importerTestProject,
						Revision: importerTestV1Rev,
					},
				},
			},
			convertTestCase{
				wantConstraint: importerTestV1Constraint,
				wantRevision:   importerTestV1Rev,
				wantVersion:    importerTestV1Tag,
			},
		},
		"missing package name": {
			govendYAML{
				Imports: []govendPackage{
					{
						Path: "",
					},
				},
			},
			convertTestCase{
				wantConvertErr: true,
			},
		},

		"missing revision": {
			govendYAML{
				Imports: []govendPackage{
					{
						Path: importerTestProject,
					},
				},
			},
			convertTestCase{
				wantConvertErr: true,
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := testCase.Exec(t, func(logger *log.Logger, sm gps.SourceManager) (*dep.Manifest, *dep.Lock, error) {
				g := newGovendImporter(logger, true, sm)
				g.yaml = testCase.yaml
				return g.convert(testProjectRoot)
			})
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
	h.TempCopy(filepath.Join(testProjectRoot, govendYAMLName), "init/govend/vendor.yml")

	projectRoot := h.Path(testProjectRoot)
	sm, err := gps.NewSourceManager(gps.SourceManagerConfig{Cachedir: h.Path(cacheDir)})
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

	govendImportOutputFile := "init/govend/golden.txt"
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
	// This is same as cmd/testdata/init/govend/vendor.yml
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
	h.TempCopy(filepath.Join(testProjectRoot, govendYAMLName), "init/govend/vendor.yml")

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
