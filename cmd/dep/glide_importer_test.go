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

func TestGlideConfig_Convert(t *testing.T) {
	testCases := map[string]struct {
		yaml                    glideYaml
		lock                    *glideLock
		expectConvertErr        bool
		matchPairedVersion      bool
		projectRoot             gps.ProjectRoot
		sourceRepo              string
		notExpectedProjectRoot  *gps.ProjectRoot
		expectedConstraint      string
		expectedRevision        *gps.Revision
		expectedVersion         string
		expectedLockCount       int
		expectedIgnoreCount     int
		expectedIgnoredPackages []string
	}{
		"project": {
			yaml: glideYaml{
				Imports: []glidePackage{
					{
						Name:       "github.com/sdboyer/deptest",
						Repository: "https://github.com/sdboyer/deptest.git",
						Reference:  "v1.0.0",
					},
				},
			},
			lock: &glideLock{
				Imports: []glideLockedPackage{
					{
						Name:       "github.com/sdboyer/deptest",
						Repository: "https://github.com/sdboyer/deptest.git",
						Reference:  "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			projectRoot:        "github.com/sdboyer/deptest",
			sourceRepo:         "https://github.com/sdboyer/deptest.git",
			matchPairedVersion: true,
			expectedConstraint: "^1.0.0",
			expectedLockCount:  1,
			expectedRevision:   gps.RevisionPtr(gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")),
			expectedVersion:    "v1.0.0",
		},
		"test project": {
			yaml: glideYaml{
				Imports: []glidePackage{
					{
						Name:      "github.com/sdboyer/deptest",
						Reference: "v1.0.0",
					},
				},
			},
			lock: &glideLock{
				Imports: []glideLockedPackage{
					{
						Name:      "github.com/sdboyer/deptest",
						Reference: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					},
				},
			},
			projectRoot:        "github.com/sdboyer/deptest",
			expectedLockCount:  1,
			expectedConstraint: "^1.0.0",
			expectedVersion:    "v1.0.0",
		},
		"with ignored package": {
			yaml: glideYaml{
				Ignores: []string{"github.com/sdboyer/deptest"},
			},
			projectRoot:             "github.com/sdboyer/deptest",
			expectedIgnoreCount:     1,
			expectedIgnoredPackages: []string{"github.com/sdboyer/deptest"},
		},
		"with exclude dir": {
			yaml: glideYaml{
				ExcludeDirs: []string{"samples"},
			},
			projectRoot:             testGlideProjectRoot,
			expectedIgnoreCount:     1,
			expectedIgnoredPackages: []string{"github.com/golang/notexist/samples"},
		},
		"exclude dir ignores mismatched package name": {
			yaml: glideYaml{
				Name:        "github.com/golang/mismatched-package-name",
				ExcludeDirs: []string{"samples"},
			},
			projectRoot:             testGlideProjectRoot,
			expectedIgnoreCount:     1,
			expectedIgnoredPackages: []string{"github.com/golang/notexist/samples"},
		},
		"bad input, empty package name": {
			yaml: glideYaml{
				Imports: []glidePackage{{Name: ""}},
			},
			projectRoot:      testGlideProjectRoot,
			expectConvertErr: true,
		},
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	for name, testCase := range testCases {
		t.Logf("Running test case: %s", name)

		g := newGlideImporter(discardLogger, true, sm)
		g.yaml = testCase.yaml

		if testCase.lock != nil {
			g.lock = testCase.lock
		}

		manifest, lock, err := g.convert(testCase.projectRoot)
		if err != nil {
			if testCase.expectConvertErr {
				continue
			}

			t.Fatal(err)
		}

		// Lock checks.
		if lock != nil && len(lock.P) != testCase.expectedLockCount {
			t.Fatalf("Expected lock to have %d project(s), got %d",
				testCase.expectedLockCount,
				len(lock.P))
		}

		// Ignored projects checks.
		if len(manifest.Ignored) != testCase.expectedIgnoreCount {
			t.Fatalf("Expected manifest to have %d ignored project(s), got %d",
				testCase.expectedIgnoreCount,
				len(manifest.Ignored))
		}

		if !equalSlice(manifest.Ignored, testCase.expectedIgnoredPackages) {
			t.Fatalf("Expected manifest to have ignore %s, got %s",
				strings.Join(testCase.expectedIgnoredPackages, ", "),
				strings.Join(manifest.Ignored, ", "))
		}

		// Constraints checks below. Skip if there is no expected constraint.
		if testCase.expectedConstraint == "" {
			continue
		}

		d, ok := manifest.Constraints[testCase.projectRoot]
		if !ok {
			t.Fatalf("Expected the manifest to have a dependency for '%s' but got none",
				testCase.projectRoot)
		}

		v := d.Constraint.String()
		if v != testCase.expectedConstraint {
			t.Fatalf("Expected manifest constraint to be %s, got %s", testCase.expectedConstraint, v)
		}

		if testCase.notExpectedProjectRoot != nil {
			_, ok := manifest.Constraints[*testCase.notExpectedProjectRoot]
			if ok {
				t.Fatalf("Expected the manifest to not have a dependency for '%s' but got none",
					*testCase.notExpectedProjectRoot)
			}
		}

		p := lock.P[0]

		if p.Ident().ProjectRoot != testCase.projectRoot {
			t.Fatalf("Expected the lock to have a project for '%s' but got '%s'",
				testCase.projectRoot,
				p.Ident().ProjectRoot)
		}

		if p.Ident().Source != testCase.sourceRepo {
			t.Fatalf("Expected locked source to be %s, got '%s'", testCase.sourceRepo, p.Ident().Source)
		}

		lv := p.Version()
		lpv, ok := lv.(gps.PairedVersion)

		if !ok {
			if testCase.matchPairedVersion {
				t.Fatalf("Expected locked version to be PairedVersion but got %T", lv)
			}

			continue
		}

		ver := lpv.String()
		if ver != testCase.expectedVersion {
			t.Fatalf("Expected locked version to be '%s', got %s", testCase.expectedVersion, ver)
		}

		if testCase.expectedRevision != nil {
			rev := lpv.Revision()
			if rev != *testCase.expectedRevision {
				t.Fatalf("Expected locked revision to be '%s', got %s", *testCase.expectedRevision,
					rev)
			}
		}
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

// equalSlice is comparing two slices for equality.
func equalSlice(a, b []string) bool {
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
