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
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

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

func TestGlideConfig_Convert(t *testing.T) {
	testCases := map[string]struct {
		yaml glideYaml
		lock glideLock
		convertTestCase
	}{
		"project": {
			glideYaml{
				Imports: []glidePackage{
					{
						Name:       "github.com/sdboyer/deptest",
						Repository: "https://github.com/sdboyer/deptest.git",
						Reference:  "v1.0.0",
					},
				},
			},
			glideLock{
				Imports: []glideLockedPackage{
					{
						Name:       "github.com/sdboyer/deptest",
						Repository: "https://github.com/sdboyer/deptest.git",
						Revision:   "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptest",
				wantSourceRepo:  "https://github.com/sdboyer/deptest.git",
				wantConstraint:  "^1.0.0",
				wantRevision:    "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				wantVersion:     "v1.0.0",
			},
		},
		"test project": {
			glideYaml{
				TestImports: []glidePackage{
					{
						Name:      "github.com/sdboyer/deptest",
						Reference: "v1.0.0",
					},
				},
			},
			glideLock{
				TestImports: []glideLockedPackage{
					{
						Name:     "github.com/sdboyer/deptest",
						Revision: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptest",
				wantConstraint:  "^1.0.0",
				wantVersion:     "v1.0.0",
				wantRevision:    "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
		},
		"revision only": {
			glideYaml{
				Imports: []glidePackage{
					{
						Name: "github.com/sdboyer/deptest",
					},
				},
			},
			glideLock{
				Imports: []glideLockedPackage{
					{
						Name:     "github.com/sdboyer/deptest",
						Revision: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptest",
				wantConstraint:  "*",
				wantRevision:    "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				wantVersion:     "v1.0.0",
			},
		},
		"with ignored package": {
			glideYaml{
				Ignores: []string{"github.com/sdboyer/deptest"},
			},
			glideLock{},
			convertTestCase{
				wantIgnored: []string{"github.com/sdboyer/deptest"},
			},
		},
		"with exclude dir": {
			glideYaml{
				ExcludeDirs: []string{"samples"},
			},
			glideLock{},
			convertTestCase{
				wantIgnored: []string{"github.com/golang/notexist/samples"},
			},
		},
		"exclude dir ignores mismatched package name": {
			glideYaml{
				Name:        "github.com/golang/mismatched-package-name",
				ExcludeDirs: []string{"samples"},
			},
			glideLock{},
			convertTestCase{
				wantIgnored: []string{"github.com/golang/notexist/samples"},
			},
		},
		"bad input, empty package name": {
			glideYaml{
				Imports: []glidePackage{{Name: ""}},
			},
			glideLock{},
			convertTestCase{
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
			g := newGlideImporter(discardLogger, true, sm)
			g.glideConfig = testCase.yaml
			g.glideLock = testCase.lock
			g.lockFound = true

			manifest, lock, convertErr := g.convert(testProjectRoot)
			err := validateConvertTestCase(testCase.convertTestCase, manifest, lock, convertErr)
			if err != nil {
				t.Fatalf("%#v", err)
			}
		})
	}
}

func TestGlideConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testProjectRoot))
	h.TempCopy(filepath.Join(testProjectRoot, glideYamlName), "init/glide/glide.yaml")
	h.TempCopy(filepath.Join(testProjectRoot, glideLockName), "init/glide/glide.lock")
	projectRoot := h.Path(testProjectRoot)

	// Capture stderr so we can verify output
	verboseOutput := &bytes.Buffer{}
	ctx.Err = log.New(verboseOutput, "", 0)

	g := newGlideImporter(ctx.Err, false, sm) // Disable verbose so that we don't print values that change each test run
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("Expected the importer to detect the glide configuration files")
	}

	m, l, err := g.Import(projectRoot, testProjectRoot)
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}

	goldenFile := "init/glide/golden.txt"
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

func TestGlideConfig_Import_MissingLockFile(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	h.TempDir(filepath.Join("src", testProjectRoot))
	h.TempCopy(filepath.Join(testProjectRoot, glideYamlName), "init/glide/glide.yaml")
	projectRoot := h.Path(testProjectRoot)

	g := newGlideImporter(ctx.Err, true, sm)
	if !g.HasDepMetadata(projectRoot) {
		t.Fatal("The glide importer should gracefully handle when only glide.yaml is present")
	}

	_, _, err = g.Import(projectRoot, testProjectRoot)
	h.Must(err)
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
			g.glideConfig = glideYaml{
				Imports: []glidePackage{pkg},
			}

			_, _, err = g.convert(testProjectRoot)
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
