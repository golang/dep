// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

func TestLookupVersionForLockedProject_MatchRevisionToTag(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	rev := gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")
	v, err := lookupVersionForLockedProject(pi, nil, rev, sm)
	h.Must(err)

	wantV := "v1.0.0"
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to be the tag paired with the manifest's pinned revision: wanted '%s', got '%s'", wantV, gotV)
	}
}

func TestLookupVersionForLockedProject_MatchRevisionToMultipleTags(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	// Both 0.8.0 and 1.0.0 use the same rev, force dep to pick the lower version
	c, _ := gps.NewSemverConstraint("<1.0.0")
	rev := gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")
	v, err := lookupVersionForLockedProject(pi, c, rev, sm)
	h.Must(err)

	wantV := "v0.8.0"
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to satisfy the manifest's semver constraint: wanted '%s', got '%s'", wantV, gotV)
	}
}

func TestLookupVersionForLockedProject_FallbackToConstraint(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	c := gps.NewBranch("master")
	rev := gps.Revision("c575196502940c07bf89fd6d95e83b999162e051")
	v, err := lookupVersionForLockedProject(pi, c, rev, sm)
	h.Must(err)

	wantV := c.String()
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to be defaulted from the manifest's branch constraint: wanted '%s', got '%s'", wantV, gotV)
	}
}

func TestLookupVersionForLockedProject_FallbackToRevision(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	rev := gps.Revision("c575196502940c07bf89fd6d95e83b999162e051")
	v, err := lookupVersionForLockedProject(pi, nil, rev, sm)
	h.Must(err)

	wantV := rev.String()
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to be the manifest's pinned revision: wanted '%s', got '%s'", wantV, gotV)
	}
}

func TestProjectExistsInLock(t *testing.T) {
	lock := &dep.Lock{}
	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	ver := gps.NewVersion("v1.0.0")
	lock.P = append(lock.P, gps.NewLockedProject(pi, ver, nil))

	cases := []struct {
		name       string
		importPath string
		want       bool
	}{
		{
			name:       "root project",
			importPath: "github.com/sdboyer/deptest",
			want:       true,
		},
		{
			name:       "sub package",
			importPath: "github.com/sdboyer/deptest/foo",
			want:       false,
		},
		{
			name:       "nonexisting project",
			importPath: "github.com/golang/notexist",
			want:       false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := projectExistsInLock(lock, gps.ProjectRoot(c.importPath))

			if result != c.want {
				t.Fatalf("projectExistsInLock result is not as want: \n\t(GOT) %v \n\t(WNT) %v", result, c.want)
			}
		})
	}
}

func TestIsVersion(t *testing.T) {
	testcases := map[string]struct {
		wantIsVersion bool
		wantVersion   gps.Version
	}{
		"v1.0.0": {wantIsVersion: true, wantVersion: gps.NewVersion("v1.0.0").Pair("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")},
		"3f4c3bea144e112a69bbe5d8d01c1b09a544253f": {wantIsVersion: false},
		"master": {wantIsVersion: false},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	for value, testcase := range testcases {
		t.Run(value, func(t *testing.T) {
			gotIsVersion, gotVersion, err := isVersion(pi, value, sm)
			h.Must(err)

			if testcase.wantIsVersion != gotIsVersion {
				t.Fatalf("unexpected isVersion result for %s: \n\t(GOT) %v \n\t(WNT) %v", value, gotIsVersion, testcase.wantIsVersion)
			}

			if testcase.wantVersion != gotVersion {
				t.Fatalf("unexpected version for %s: \n\t(GOT) %v \n\t(WNT) %v", value, gotVersion, testcase.wantVersion)
			}
		})
	}
}

// convertTestCase is a common set of validations applied to the result
// of an importer converting from an external config format to dep's.
type convertTestCase struct {
	wantConvertErr      bool
	projectRoot         gps.ProjectRoot
	wantSourceRepo      string
	wantConstraint      string
	wantRevision        gps.Revision
	wantVersion         string
	wantLockCount       int
	wantIgnoreCount     int
	wantIgnoredPackages []string
}

// validateConvertTestCase returns an error if any of the importer's
// conversion validations failed.
func validateConvertTestCase(testCase *convertTestCase, manifest *dep.Manifest, lock *dep.Lock, convertErr error) error {
	if testCase.wantConvertErr {
		if convertErr == nil {
			return errors.New("Expected the conversion to fail, but it did not return an error")
		}
		return nil
	}

	if convertErr != nil {
		return errors.Wrap(convertErr, "Expected the conversion to pass, but it returned an error")
	}

	// Ignored projects checks.
	if len(manifest.Ignored) != testCase.wantIgnoreCount {
		return errors.Errorf("Expected manifest to have %d ignored project(s), got %d",
			testCase.wantIgnoreCount,
			len(manifest.Ignored))
	}

	if !equalSlice(manifest.Ignored, testCase.wantIgnoredPackages) {
		return errors.Errorf("Expected manifest to have ignore %s, got %s",
			strings.Join(testCase.wantIgnoredPackages, ", "),
			strings.Join(manifest.Ignored, ", "))
	}

	// Constraints checks below.
	if testCase.wantConstraint != "" {
		d, ok := manifest.Constraints[testCase.projectRoot]
		if !ok {
			return errors.Errorf("Expected the manifest to have a dependency for '%s' but got none",
				testCase.projectRoot)
		}

		v := d.Constraint.String()
		if v != testCase.wantConstraint {
			return errors.Errorf("Expected manifest constraint to be %s, got %s", testCase.wantConstraint, v)
		}
	}

	// Lock checks.
	if lock != nil {
		if len(lock.P) != testCase.wantLockCount {
			return errors.Errorf("Expected lock to have %d project(s), got %d",
				testCase.wantLockCount,
				len(lock.P))
		}

		p := lock.P[0]

		if p.Ident().ProjectRoot != testCase.projectRoot {
			return errors.Errorf("Expected the lock to have a project for '%s' but got '%s'",
				testCase.projectRoot,
				p.Ident().ProjectRoot)
		}

		if p.Ident().Source != testCase.wantSourceRepo {
			return errors.Errorf("Expected locked source to be %s, got '%s'", testCase.wantSourceRepo, p.Ident().Source)
		}

		// Break down the locked "version" into a version (optional) and revision
		var gotVersion string
		var gotRevision gps.Revision
		if lpv, ok := p.Version().(gps.PairedVersion); ok {
			gotVersion = lpv.String()
			gotRevision = lpv.Revision()
		} else if lr, ok := p.Version().(gps.Revision); ok {
			gotRevision = lr
		} else {
			return errors.New("could not determine the type of the locked version")
		}

		if gotRevision != testCase.wantRevision {
			return errors.Errorf("unexpected locked revision : \n\t(GOT) %v \n\t(WNT) %v",
				gotRevision,
				testCase.wantRevision)
		}
		if gotVersion != testCase.wantVersion {
			return errors.Errorf("unexpected locked version: \n\t(GOT) %v \n\t(WNT) %v",
				gotVersion,
				testCase.wantVersion)
		}
	}
	return nil
}
