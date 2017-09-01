// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"path/filepath"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
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
						Name:       importerTestProject,
						Repository: importerTestProjectSrc,
						Reference:  importerTestV2Branch,
					},
				},
			},
			glideLock{
				Imports: []glideLockedPackage{
					{
						Name:       importerTestProject,
						Repository: importerTestProjectSrc,
						Revision:   importerTestV2PatchRev,
					},
				},
			},
			convertTestCase{
				wantSourceRepo: importerTestProjectSrc,
				wantConstraint: importerTestV2Branch,
				wantRevision:   importerTestV2PatchRev,
				wantVersion:    importerTestV2PatchTag,
			},
		},
		"test project": {
			glideYaml{
				Imports: []glidePackage{
					{
						Name:       importerTestProject,
						Repository: importerTestProjectSrc,
						Reference:  importerTestV2Branch,
					},
				},
			},
			glideLock{
				Imports: []glideLockedPackage{
					{
						Name:       importerTestProject,
						Repository: importerTestProjectSrc,
						Revision:   importerTestV2PatchRev,
					},
				},
			},
			convertTestCase{
				wantSourceRepo: importerTestProjectSrc,
				wantConstraint: importerTestV2Branch,
				wantRevision:   importerTestV2PatchRev,
				wantVersion:    importerTestV2PatchTag,
			},
		},
		"yaml only": {
			glideYaml{
				Imports: []glidePackage{
					{
						Name:       importerTestProject,
						Repository: importerTestProjectSrc,
						Reference:  importerTestV2Branch,
					},
				},
			},
			glideLock{},
			convertTestCase{
				wantSourceRepo: importerTestProjectSrc,
				wantConstraint: importerTestV2Branch,
			},
		},
		"ignored package": {
			glideYaml{
				Ignores: []string{importerTestProject},
			},
			glideLock{},
			convertTestCase{
				wantIgnored: []string{importerTestProject},
			},
		},
		"exclude dir": {
			glideYaml{
				ExcludeDirs: []string{"samples"},
			},
			glideLock{},
			convertTestCase{
				wantIgnored: []string{testProjectRoot + "/samples"},
			},
		},
		"exclude dir ignores mismatched package name": {
			glideYaml{
				Name:        "github.com/golang/mismatched-package-name",
				ExcludeDirs: []string{"samples"},
			},
			glideLock{},
			convertTestCase{
				wantIgnored: []string{testProjectRoot + "/samples"},
			},
		},
		"missing package name": {
			glideYaml{
				Imports: []glidePackage{{Name: ""}},
			},
			glideLock{},
			convertTestCase{
				wantConvertErr: true,
			},
		},
		"warn unused os field": {
			glideYaml{
				Imports: []glidePackage{
					{
						Name: importerTestProject,
						OS:   "windows",
					},
				}},
			glideLock{},
			convertTestCase{
				wantConstraint: "*",
				wantWarning:    "specified an os",
			},
		},
		"warn unused arch field": {
			glideYaml{
				Imports: []glidePackage{
					{
						Name: importerTestProject,
						Arch: "i686",
					},
				}},
			glideLock{},
			convertTestCase{
				wantConstraint: "*",
				wantWarning:    "specified an arch",
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := testCase.Exec(t, func(logger *log.Logger, sm gps.SourceManager) (*dep.Manifest, *dep.Lock, error) {
				g := newGlideImporter(logger, true, sm)
				g.glideConfig = testCase.yaml
				g.glideLock = testCase.lock
				return g.convert(testProjectRoot)
			})
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
