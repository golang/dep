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

func TestGoDepConfig_Convert(t *testing.T) {
	testCases := map[string]struct {
		json                   godepJSON
		expectConvertErr       bool
		matchPairedVersion     bool
		projectRoot            gps.ProjectRoot
		notExpectedProjectRoot *gps.ProjectRoot
		expectedConstraint     string
		expectedRevision       *gps.Revision
		expectedVersion        string
		expectedLockCount      int
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
			matchPairedVersion: true,
			projectRoot:        gps.ProjectRoot("github.com/sdboyer/deptest"),
			expectedConstraint: "^0.8.0",
			expectedRevision:   gps.RevisionPtr(gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")),
			expectedVersion:    "v0.8.0",
			expectedLockCount:  1,
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
			projectRoot:        gps.ProjectRoot("github.com/sdboyer/deptest"),
			matchPairedVersion: false,
			expectedConstraint: ">=1.12.0, <=12.0.0-g2fd980e",
			expectedLockCount:  1,
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
			projectRoot:        gps.ProjectRoot("github.com/sdboyer/deptest"),
			matchPairedVersion: true,
			expectedConstraint: "^1.0.0",
			expectedRevision:   gps.RevisionPtr(gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")),
			expectedVersion:    "v1.0.0",
			expectedLockCount:  1,
		},
		"bad input - empty package name": {
			json: godepJSON{
				Imports: []godepPackage{{ImportPath: ""}},
			},
			expectConvertErr: true,
		},
		"bad input - empty revision": {
			json: godepJSON{
				Imports: []godepPackage{
					{
						ImportPath: "github.com/sdboyer/deptest",
					},
				},
			},
			expectConvertErr: true,
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
			projectRoot:            gps.ProjectRoot("github.com/sdboyer/deptest"),
			notExpectedProjectRoot: gps.ProjectRootPtr(gps.ProjectRoot("github.com/sdboyer/deptest/foo")),
			expectedLockCount:      1,
			expectedConstraint:     "^1.0.0",
			expectedVersion:        "v1.0.0",
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

		g := newGodepImporter(discardLogger, true, sm)
		g.json = testCase.json

		manifest, lock, err := g.convert(testCase.projectRoot)
		if err != nil {
			if testCase.expectConvertErr {
				continue
			}

			t.Fatal(err)
		}

		if len(lock.P) != testCase.expectedLockCount {
			t.Fatalf("Expected lock to have %d project(s), got %d",
				testCase.expectedLockCount,
				len(lock.P))
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
				t.Fatalf("Expected locked revision to be '%s', got %s",
					*testCase.expectedRevision,
					rev)
			}
		}
	}
}

func TestGodepConfig_Import(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	h.TempDir("src")
	h.TempDir(filepath.Join("src", testGodepProjectRoot))
	h.TempCopy(filepath.Join(testGodepProjectRoot, godepPath), "godep/Godeps.json")

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

	h.TempCopy(filepath.Join(testGodepProjectRoot, godepPath), "godep/Godeps.json")

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
