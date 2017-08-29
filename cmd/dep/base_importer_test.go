// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"sort"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

// convertTestCase is a common set of validations applied to the result
// of an importer converting from an external config format to dep's.
type convertTestCase struct {
	defaultConstraintFromLock bool
	wantConvertErr            bool
	wantProjectRoot           gps.ProjectRoot
	wantSourceRepo            string
	wantConstraint            string
	wantRevision              gps.Revision
	wantVersion               string
	wantIgnored               []string
}

func TestBaseImporter_IsTag(t *testing.T) {
	testcases := map[string]struct {
		wantIsTag bool
		wantTag   gps.Version
	}{
		// TODO(carolynvs): need repo with a non-semver tag
		"v1.0.0": {wantIsTag: true, wantTag: gps.NewVersion("v1.0.0").Pair("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")},
		"3f4c3bea144e112a69bbe5d8d01c1b09a544253f": {wantIsTag: false},
		"master": {wantIsTag: false},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: "github.com/sdboyer/deptest"}
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	for value, testcase := range testcases {
		t.Run(value, func(t *testing.T) {
			i := newBaseImporter(discardLogger, false, sm)

			gotIsTag, gotTag, err := i.isTag(pi, value)
			h.Must(err)

			if testcase.wantIsTag != gotIsTag {
				t.Fatalf("unexpected isVersion result for %v: \n\t(GOT) %v \n\t(WNT) %v", value, gotIsTag, testcase.wantIsTag)
			}

			if testcase.wantTag != gotTag {
				t.Fatalf("unexpected version for %v: \n\t(GOT) %v \n\t(WNT) %v", value, gotTag, testcase.wantTag)
			}
		})
	}
}

func TestBaseImporter_LookupVersionForLockedProject(t *testing.T) {
	lessThanV1, _ := gps.NewSemverConstraint("<1.0.0")

	testcases := map[string]struct {
		revision    gps.Revision
		constraint  gps.Constraint
		wantVersion string
	}{
		"match revision to tag": {
			revision:    "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			wantVersion: "v1.0.0",
		},
		"match revision to multiple tags": {
			revision:    "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			constraint:  lessThanV1,
			wantVersion: "v0.8.0",
		},
		"fallback to branch constraint": {
			revision:    "c575196502940c07bf89fd6d95e83b999162e051",
			constraint:  gps.NewBranch("master"),
			wantVersion: "master",
		},
		"fallback to revision": {
			revision:    "c575196502940c07bf89fd6d95e83b999162e051",
			wantVersion: "c575196502940c07bf89fd6d95e83b999162e051",
		},
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: "github.com/sdboyer/deptest"}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			i := newBaseImporter(discardLogger, false, sm)
			v, err := i.lookupVersionForLockedProject(pi, tc.constraint, tc.revision)
			h.Must(err)

			gotVersion := v.String()
			if gotVersion != tc.wantVersion {
				t.Fatalf("unexpected locked version: \n\t(GOT) %v\n\t(WNT) %v", gotVersion, tc.wantVersion)
			}
		})
	}
}

func TestBaseImporter_ImportProjects(t *testing.T) {
	testcases := map[string]struct {
		convertTestCase
		projects []importedPackage
	}{
		"constraint only": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptestdos",
				wantConstraint:  "master",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptestdos",
					ConstraintHint: "master", // make a repo with a tag that isn't semver, e.g. beta1
				},
			},
		},
		"untagged revision ignores tag constraint": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptestdos",
				wantConstraint:  "*",
				wantRevision:    "5eff28fbbf20a75c9ea1140a3d71338648dad508",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptestdos",
					LockHint:       "5eff28fbbf20a75c9ea1140a3d71338648dad508",
					ConstraintHint: "TODO", // make a repo with a tag that isn't semver, e.g. beta1
				},
			},
		},
		"untagged revision ignores range constraint": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptestdos",
				wantConstraint:  "*",
				wantRevision:    "5eff28fbbf20a75c9ea1140a3d71338648dad508",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptestdos",
					LockHint:       "5eff28fbbf20a75c9ea1140a3d71338648dad508",
					ConstraintHint: "2.0.0",
				},
			},
		},
		"untagged revision keeps branch constraint": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptestdos",
				wantConstraint:  "master",
				wantVersion:     "master",
				wantRevision:    "5eff28fbbf20a75c9ea1140a3d71338648dad508",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptestdos",
					LockHint:       "5eff28fbbf20a75c9ea1140a3d71338648dad508",
					ConstraintHint: "master",
				},
			},
		},
		"HEAD revisions default to the matching branch": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptestdos",
				wantConstraint:  "*",
				wantVersion:     "master",
				wantRevision:    "a0196baa11ea047dd65037287451d36b861b00ea",
			},
			[]importedPackage{
				{
					Name:     "github.com/sdboyer/deptestdos",
					LockHint: "a0196baa11ea047dd65037287451d36b861b00ea",
				},
			},
		},
		"Semver tagged revisions default to ^VERSION": {
			convertTestCase{
				defaultConstraintFromLock: true,
				wantProjectRoot:           "github.com/sdboyer/deptest",
				wantConstraint:            "^1.0.0",
				wantVersion:               "v1.0.0",
				wantRevision:              "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
			[]importedPackage{
				{
					Name:     "github.com/sdboyer/deptest",
					LockHint: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				},
			},
		},
		"Semver lock hint defaults constraint to ^VERSION": {
			convertTestCase{
				defaultConstraintFromLock: true,
				wantProjectRoot:           "github.com/sdboyer/deptest",
				wantConstraint:            "^1.0.0",
				wantVersion:               "v1.0.0",
				wantRevision:              "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
			[]importedPackage{
				{
					Name:     "github.com/sdboyer/deptest",
					LockHint: "v1.0.0",
				},
			},
		},
		"Semver constraint hint": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptest",
				wantConstraint:  ">0.8.0",
				wantVersion:     "v1.0.0",
				wantRevision:    "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptest",
					LockHint:       "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					ConstraintHint: ">0.8.0",
				},
			},
		},
		"Revision constraints are ignored": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptest",
				wantConstraint:  "*",
				wantVersion:     "v1.0.0",
				wantRevision:    "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptest",
					LockHint:       "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
					ConstraintHint: "ff2948a2ac8f538c4ecd55962e919d1e13e74baf",
				},
			},
		},
		"Branch constraint hint": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptest",
				wantConstraint:  "master",
				wantVersion:     "v0.8.1",
				wantRevision:    "3f4c3bea144e112a69bbe5d8d01c1b09a544253f",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptest",
					LockHint:       "3f4c3bea144e112a69bbe5d8d01c1b09a544253f",
					ConstraintHint: "master",
				},
			},
		},
		"Non-matching semver constraint is ignored": {
			convertTestCase{
				wantProjectRoot: "github.com/sdboyer/deptest",
				wantConstraint:  "*",
				wantVersion:     "v0.8.1",
				wantRevision:    "3f4c3bea144e112a69bbe5d8d01c1b09a544253f",
			},
			[]importedPackage{
				{
					Name:           "github.com/sdboyer/deptest",
					LockHint:       "3f4c3bea144e112a69bbe5d8d01c1b09a544253f",
					ConstraintHint: "^2.0.0",
				},
			},
		},
		"consolidate subpackages under root": {
			convertTestCase{
				wantProjectRoot: "github.com/carolynvs/deptest-subpkg",
				wantConstraint:  "master",
				wantVersion:     "master",
				wantRevision:    "6c41d90f78bb1015696a2ad591debfa8971512d5",
			},
			[]importedPackage{
				{
					Name:           "github.com/carolynvs/deptest-subpkg/subby",
					ConstraintHint: "master",
				},
				{
					Name:     "github.com/carolynvs/deptest-subpkg",
					LockHint: "6c41d90f78bb1015696a2ad591debfa8971512d5",
				},
			},
		},
		"ignore duplicate packages": {
			convertTestCase{
				wantProjectRoot: "github.com/carolynvs/deptest-subpkg",
				wantConstraint:  "*",
				wantRevision:    "6c41d90f78bb1015696a2ad591debfa8971512d5",
			},
			[]importedPackage{
				{
					Name:     "github.com/carolynvs/deptest-subpkg/supkg1",
					LockHint: "6c41d90f78bb1015696a2ad591debfa8971512d5", // first wins
				},
				{
					Name:     "github.com/carolynvs/deptest-subpkg/supkg2",
					LockHint: "b90e5f3a888585ea5df81d3fe0c81fc6e3e3d70b",
				},
			},
		},

		// TODO: classify v1.12.0-gabc123 testcase
		// TODO: unhelpful constraints like "HEAD"
		// TODO: unhelpful locks like a revision that doesn't exist
		// * Versions that don't satisfy the constraint, drop the constraint.
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			i := newBaseImporter(discardLogger, false, sm)

			convertErr := i.importPackages(testcase.projects, testcase.defaultConstraintFromLock)
			err := validateConvertTestCase(testcase.convertTestCase, i.manifest, i.lock, convertErr)
			if err != nil {
				t.Fatalf("%#v", err)
			}
		})
	}
}

// validateConvertTestCase returns an error if any of the importer's
// conversion validations failed.
func validateConvertTestCase(testCase convertTestCase, manifest *dep.Manifest, lock *dep.Lock, convertErr error) error {
	if testCase.wantConvertErr {
		if convertErr == nil {
			return errors.New("Expected the conversion to fail, but it did not return an error")
		}
		return nil
	}

	if convertErr != nil {
		return errors.Wrap(convertErr, "Expected the conversion to pass, but it returned an error")
	}

	if !equalSlice(manifest.Ignored, testCase.wantIgnored) {
		return errors.Errorf("unexpected set of ignored projects: \n\t(GOT) %v \n\t(WNT) %v",
			manifest.Ignored, testCase.wantIgnored)
	}

	wantConstraintCount := 0
	if testCase.wantConstraint != "" {
		wantConstraintCount = 1
	}
	gotConstraintCount := len(manifest.Constraints)
	if gotConstraintCount != wantConstraintCount {
		return errors.Errorf("unexpected number of constraints: \n\t(GOT) %v \n\t(WNT) %v",
			gotConstraintCount, wantConstraintCount)
	}

	if testCase.wantConstraint != "" {
		d, ok := manifest.Constraints[testCase.wantProjectRoot]
		if !ok {
			return errors.Errorf("Expected the manifest to have a dependency for '%v'",
				testCase.wantProjectRoot)
		}

		gotConstraint := d.Constraint.String()
		if gotConstraint != testCase.wantConstraint {
			return errors.Errorf("unexpected constraint: \n\t(GOT) %v \n\t(WNT) %v",
				gotConstraint, testCase.wantConstraint)
		}

	}

	// Lock checks.
	wantLockCount := 0
	if testCase.wantRevision != "" {
		wantLockCount = 1
	}
	gotLockCount := 0
	if lock != nil {
		gotLockCount = len(lock.P)
	}
	if gotLockCount != wantLockCount {
		return errors.Errorf("unexpected number of locked projects: \n\t(GOT) %v \n\t(WNT) %v",
			gotLockCount, wantLockCount)
	}

	if testCase.wantRevision != "" {
		lp := lock.P[0]

		gotProjectRoot := lp.Ident().ProjectRoot
		if gotProjectRoot != testCase.wantProjectRoot {
			return errors.Errorf("unexpected root project in lock: \n\t(GOT) %v \n\t(WNT) %v",
				gotProjectRoot, testCase.wantProjectRoot)
		}

		gotSource := lp.Ident().Source
		if gotSource != testCase.wantSourceRepo {
			return errors.Errorf("unexpected source repository: \n\t(GOT) %v \n\t(WNT) %v",
				gotSource, testCase.wantSourceRepo)
		}

		// Break down the locked "version" into a version (optional) and revision
		var gotVersion string
		var gotRevision gps.Revision
		if lpv, ok := lp.Version().(gps.PairedVersion); ok {
			gotVersion = lpv.String()
			gotRevision = lpv.Revision()
		} else if lr, ok := lp.Version().(gps.Revision); ok {
			gotRevision = lr
		} else {
			return errors.New("could not determine the type of the locked version")
		}

		if gotRevision != testCase.wantRevision {
			return errors.Errorf("unexpected locked revision: \n\t(GOT) %v \n\t(WNT) %v",
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

// equalSlice is comparing two string slices for equality.
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

	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
