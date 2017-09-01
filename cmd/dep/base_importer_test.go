// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"log"
	"sort"
	"strings"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

const (
	importerTestProject              = "github.com/carolynvs/deptest-importers"
	importerTestProjectSrc           = "https://github.com/carolynvs/deptest-importers.git"
	importerTestUntaggedRev          = "9b670d143bfb4a00f7461451d5c4a62f80e9d11d"
	importerTestUntaggedRevAbbrv     = "v1.0.0-1-g9b670d1"
	importerTestBeta1Tag             = "beta1"
	importerTestBeta1Rev             = "7913ab26988c6fb1e16225f845a178e8849dd254"
	importerTestV2Branch             = "v2"
	importerTestV2Rev                = "45dcf5a09c64b48b6e836028a3bc672b19b9d11d"
	importerTestV2PatchTag           = "v2.0.0-alpha1"
	importerTestV2PatchRev           = "347760b50204948ea63e531dd6560e56a9adde8f"
	importerTestV1Tag                = "v1.0.0"
	importerTestV1Rev                = "d0c29640b17f77426b111f4c1640d716591aa70e"
	importerTestV1PatchTag           = "v1.0.2"
	importerTestV1PatchRev           = "788963efe22e3e6e24c776a11a57468bb2fcd780"
	importerTestV1Constraint         = "^1.0.0"
	importerTestMultiTaggedRev       = "34cf993cc346f65601fe4356dd68bd54d20a1bfe"
	importerTestMultiTaggedSemverTag = "v1.0.4"
	importerTestMultiTaggedPlainTag  = "stable"
)

func TestBaseImporter_IsTag(t *testing.T) {
	testcases := map[string]struct {
		input     string
		wantIsTag bool
		wantTag   gps.Version
	}{
		"non-semver tag": {
			input:     importerTestBeta1Tag,
			wantIsTag: true,
			wantTag:   gps.NewVersion(importerTestBeta1Tag).Pair(importerTestBeta1Rev),
		},
		"semver-tag": {
			input:     importerTestV1PatchTag,
			wantIsTag: true,
			wantTag:   gps.NewVersion(importerTestV1PatchTag).Pair(importerTestV1PatchRev)},
		"untagged revision": {
			input:     importerTestUntaggedRev,
			wantIsTag: false,
		},
		"branch name": {
			input:     importerTestV2Branch,
			wantIsTag: false,
		},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: importerTestProject}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()
			h.Parallel()

			ctx := newTestContext(h)
			sm, err := ctx.SourceManager()
			h.Must(err)
			defer sm.Release()

			i := newBaseImporter(discardLogger, false, sm)
			gotIsTag, gotTag, err := i.isTag(pi, tc.input)
			h.Must(err)

			if tc.wantIsTag != gotIsTag {
				t.Fatalf("unexpected isTag result for %v: \n\t(GOT) %v \n\t(WNT) %v",
					tc.input, gotIsTag, tc.wantIsTag)
			}

			if tc.wantTag != gotTag {
				t.Fatalf("unexpected tag for %v: \n\t(GOT) %v \n\t(WNT) %v",
					tc.input, gotTag, tc.wantTag)
			}
		})
	}
}

func TestBaseImporter_LookupVersionForLockedProject(t *testing.T) {
	testcases := map[string]struct {
		revision    gps.Revision
		constraint  gps.Constraint
		wantVersion string
	}{
		"match revision to tag": {
			revision:    importerTestV1PatchRev,
			wantVersion: importerTestV1PatchTag,
		},
		"match revision with multiple tags using constraint": {
			revision:    importerTestMultiTaggedRev,
			constraint:  gps.NewVersion(importerTestMultiTaggedPlainTag),
			wantVersion: importerTestMultiTaggedPlainTag,
		},
		"revision with multiple tags with no constraint defaults to best match": {
			revision:    importerTestMultiTaggedRev,
			wantVersion: importerTestMultiTaggedSemverTag,
		},
		"revision with multiple tags with nonmatching constraint defaults to best match": {
			revision:    importerTestMultiTaggedRev,
			constraint:  gps.NewVersion("thismatchesnothing"),
			wantVersion: importerTestMultiTaggedSemverTag,
		},
		"untagged revision fallback to branch constraint": {
			revision:    importerTestUntaggedRev,
			constraint:  gps.NewBranch("master"),
			wantVersion: "master",
		},
		"fallback to revision": {
			revision:    importerTestUntaggedRev,
			wantVersion: importerTestUntaggedRev,
		},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: importerTestProject}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()
			h.Parallel()

			ctx := newTestContext(h)
			sm, err := ctx.SourceManager()
			h.Must(err)
			defer sm.Release()

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
		"tag constraints are ignored": {
			convertTestCase{
				wantConstraint: "*",
				wantVersion:    importerTestBeta1Tag,
				wantRevision:   importerTestBeta1Rev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestBeta1Rev,
					ConstraintHint: importerTestBeta1Tag,
				},
			},
		},
		"tag lock hints lock to tagged revision": {
			convertTestCase{
				wantConstraint: "*",
				wantVersion:    importerTestBeta1Tag,
				wantRevision:   importerTestBeta1Rev,
			},
			[]importedPackage{
				{
					Name:     importerTestProject,
					LockHint: importerTestBeta1Tag,
				},
			},
		},
		"untagged revision ignores range constraint": {
			convertTestCase{
				wantConstraint: "*",
				wantRevision:   importerTestUntaggedRev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestUntaggedRev,
					ConstraintHint: importerTestV1Constraint,
				},
			},
		},
		"untagged revision keeps branch constraint": {
			convertTestCase{
				wantConstraint: "master",
				wantVersion:    "master",
				wantRevision:   importerTestUntaggedRev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestUntaggedRev,
					ConstraintHint: "master",
				},
			},
		},
		"HEAD revisions default constraint to the matching branch": {
			convertTestCase{
				defaultConstraintFromLock: true,
				wantConstraint:            importerTestV2Branch,
				wantVersion:               importerTestV2Branch,
				wantRevision:              importerTestV2Rev,
			},
			[]importedPackage{
				{
					Name:     importerTestProject,
					LockHint: importerTestV2Rev,
				},
			},
		},
		"Semver tagged revisions default to ^VERSION": {
			convertTestCase{
				defaultConstraintFromLock: true,
				wantConstraint:            importerTestV1Constraint,
				wantVersion:               importerTestV1Tag,
				wantRevision:              importerTestV1Rev,
			},
			[]importedPackage{
				{
					Name:     importerTestProject,
					LockHint: importerTestV1Rev,
				},
			},
		},
		"Semver lock hint defaults constraint to ^VERSION": {
			convertTestCase{
				defaultConstraintFromLock: true,
				wantConstraint:            importerTestV1Constraint,
				wantVersion:               importerTestV1Tag,
				wantRevision:              importerTestV1Rev,
			},
			[]importedPackage{
				{
					Name:     importerTestProject,
					LockHint: importerTestV1Tag,
				},
			},
		},
		"Semver constraint hint": {
			convertTestCase{
				wantConstraint: importerTestV1Constraint,
				wantVersion:    importerTestV1PatchTag,
				wantRevision:   importerTestV1PatchRev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestV1PatchRev,
					ConstraintHint: importerTestV1Constraint,
				},
			},
		},
		"Semver prerelease lock hint": {
			convertTestCase{
				wantConstraint: importerTestV2Branch,
				wantVersion:    importerTestV2PatchTag,
				wantRevision:   importerTestV2PatchRev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestV2PatchRev,
					ConstraintHint: importerTestV2Branch,
				},
			},
		},
		"Revision constraints are ignored": {
			convertTestCase{
				wantConstraint: "*",
				wantVersion:    importerTestV1Tag,
				wantRevision:   importerTestV1Rev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestV1Rev,
					ConstraintHint: importerTestV1Rev,
				},
			},
		},
		"Branch constraint hint": {
			convertTestCase{
				wantConstraint: "master",
				wantVersion:    importerTestV1Tag,
				wantRevision:   importerTestV1Rev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestV1Rev,
					ConstraintHint: "master",
				},
			},
		},
		"Non-matching semver constraint is ignored": {
			convertTestCase{
				wantConstraint: "*",
				wantVersion:    importerTestV1Tag,
				wantRevision:   importerTestV1Rev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestV1Rev,
					ConstraintHint: "^2.0.0",
				},
			},
		},
		"git describe constraint is ignored": {
			convertTestCase{
				wantConstraint: "*",
				wantRevision:   importerTestUntaggedRev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject,
					LockHint:       importerTestUntaggedRev,
					ConstraintHint: importerTestUntaggedRevAbbrv,
				},
			},
		},
		"consolidate subpackages under root": {
			convertTestCase{
				wantConstraint: "master",
				wantVersion:    "master",
				wantRevision:   importerTestUntaggedRev,
			},
			[]importedPackage{
				{
					Name:           importerTestProject + "/subpkA",
					ConstraintHint: "master",
				},
				{
					Name:     importerTestProject,
					LockHint: importerTestUntaggedRev,
				},
			},
		},
		"ignore duplicate packages": {
			convertTestCase{
				wantConstraint: "*",
				wantRevision:   importerTestUntaggedRev,
			},
			[]importedPackage{
				{
					Name:     importerTestProject + "/subpkgA",
					LockHint: importerTestUntaggedRev, // first wins
				},
				{
					Name:     importerTestProject + "/subpkgB",
					LockHint: importerTestV1Rev,
				},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tc.Exec(t, func(logger *log.Logger, sm gps.SourceManager) (*dep.Manifest, *dep.Lock, error) {
				i := newBaseImporter(logger, true, sm)
				convertErr := i.importPackages(tc.projects, tc.defaultConstraintFromLock)
				return i.manifest, i.lock, convertErr
			})
			if err != nil {
				t.Fatalf("%#v", err)
			}
		})
	}
}

// convertTestCase is a common set of validations applied to the result
// of an importer converting from an external config format to dep's.
type convertTestCase struct {
	defaultConstraintFromLock bool
	wantConvertErr            bool
	wantSourceRepo            string
	wantConstraint            string
	wantRevision              gps.Revision
	wantVersion               string
	wantIgnored               []string
	wantWarning               string
}

func (tc convertTestCase) Exec(t *testing.T, convert func(logger *log.Logger, sm gps.SourceManager) (*dep.Manifest, *dep.Lock, error)) error {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	// Capture stderr so we can verify warnings
	output := &bytes.Buffer{}
	ctx.Err = log.New(output, "", 0)

	manifest, lock, convertErr := convert(ctx.Err, sm)
	return tc.validate(manifest, lock, convertErr, output)
}

// validate returns an error if any of the testcase validations failed.
func (tc convertTestCase) validate(manifest *dep.Manifest, lock *dep.Lock, convertErr error, output *bytes.Buffer) error {
	if tc.wantConvertErr {
		if convertErr == nil {
			return errors.New("Expected the conversion to fail, but it did not return an error")
		}
		return nil
	}

	if convertErr != nil {
		return errors.Wrap(convertErr, "Expected the conversion to pass, but it returned an error")
	}

	if !equalSlice(manifest.Ignored, tc.wantIgnored) {
		return errors.Errorf("unexpected set of ignored projects: \n\t(GOT) %v \n\t(WNT) %v",
			manifest.Ignored, tc.wantIgnored)
	}

	wantConstraintCount := 0
	if tc.wantConstraint != "" {
		wantConstraintCount = 1
	}
	gotConstraintCount := len(manifest.Constraints)
	if gotConstraintCount != wantConstraintCount {
		return errors.Errorf("unexpected number of constraints: \n\t(GOT) %v \n\t(WNT) %v",
			gotConstraintCount, wantConstraintCount)
	}

	if tc.wantConstraint != "" {
		d, ok := manifest.Constraints[importerTestProject]
		if !ok {
			return errors.Errorf("Expected the manifest to have a dependency for '%v'",
				importerTestProject)
		}

		gotConstraint := d.Constraint.String()
		if gotConstraint != tc.wantConstraint {
			return errors.Errorf("unexpected constraint: \n\t(GOT) %v \n\t(WNT) %v",
				gotConstraint, tc.wantConstraint)
		}

	}

	// Lock checks.
	wantLockCount := 0
	if tc.wantRevision != "" {
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

	if tc.wantRevision != "" {
		lp := lock.P[0]

		gotProjectRoot := lp.Ident().ProjectRoot
		if gotProjectRoot != importerTestProject {
			return errors.Errorf("unexpected root project in lock: \n\t(GOT) %v \n\t(WNT) %v",
				gotProjectRoot, importerTestProject)
		}

		gotSource := lp.Ident().Source
		if gotSource != tc.wantSourceRepo {
			return errors.Errorf("unexpected source repository: \n\t(GOT) %v \n\t(WNT) %v",
				gotSource, tc.wantSourceRepo)
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

		if gotRevision != tc.wantRevision {
			return errors.Errorf("unexpected locked revision: \n\t(GOT) %v \n\t(WNT) %v",
				gotRevision,
				tc.wantRevision)
		}
		if gotVersion != tc.wantVersion {
			return errors.Errorf("unexpected locked version: \n\t(GOT) %v \n\t(WNT) %v",
				gotVersion,
				tc.wantVersion)
		}
	}

	if tc.wantWarning != "" {
		if !strings.Contains(output.String(), tc.wantWarning) {
			return errors.Errorf("Expected the output to include the warning '%s'", tc.wantWarning)
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
