// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package base

import (
	"log"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/importers/importertest"
	"github.com/golang/dep/internal/test"
)

func TestBaseImporter_IsTag(t *testing.T) {
	testcases := map[string]struct {
		input     string
		wantIsTag bool
		wantTag   gps.Version
	}{
		"non-semver tag": {
			input:     importertest.Beta1Tag,
			wantIsTag: true,
			wantTag:   gps.NewVersion(importertest.Beta1Tag).Pair(importertest.Beta1Rev),
		},
		"semver-tag": {
			input:     importertest.V1PatchTag,
			wantIsTag: true,
			wantTag:   gps.NewVersion(importertest.V1PatchTag).Pair(importertest.V1PatchRev)},
		"untagged revision": {
			input:     importertest.UntaggedRev,
			wantIsTag: false,
		},
		"branch name": {
			input:     importertest.V2Branch,
			wantIsTag: false,
		},
		"empty": {
			input:     "",
			wantIsTag: false,
		},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: importertest.Project}

	for name, tc := range testcases {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()
			// Disable parallel tests until we can resolve this error on the Windows builds:
			// "remote repository at https://github.com/carolynvs/deptest-importers does not exist, or is inaccessible"
			//h.Parallel()

			ctx := importertest.NewTestContext(h)
			sm, err := ctx.SourceManager()
			h.Must(err)
			defer sm.Release()

			i := NewImporter(ctx.Err, ctx.Verbose, sm)
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
			revision:    importertest.V1PatchRev,
			wantVersion: importertest.V1PatchTag,
		},
		"match revision with multiple tags using constraint": {
			revision:    importertest.MultiTaggedRev,
			constraint:  gps.NewVersion(importertest.MultiTaggedPlainTag),
			wantVersion: importertest.MultiTaggedPlainTag,
		},
		"revision with multiple tags with no constraint defaults to best match": {
			revision:    importertest.MultiTaggedRev,
			wantVersion: importertest.MultiTaggedSemverTag,
		},
		"revision with multiple tags with nonmatching constraint defaults to best match": {
			revision:    importertest.MultiTaggedRev,
			constraint:  gps.NewVersion("thismatchesnothing"),
			wantVersion: importertest.MultiTaggedSemverTag,
		},
		"untagged revision fallback to branch constraint": {
			revision:    importertest.UntaggedRev,
			constraint:  gps.NewBranch("master"),
			wantVersion: "master",
		},
		"fallback to revision": {
			revision:    importertest.UntaggedRev,
			wantVersion: importertest.UntaggedRev,
		},
	}

	pi := gps.ProjectIdentifier{ProjectRoot: importertest.Project}

	for name, tc := range testcases {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			h := test.NewHelper(t)
			defer h.Cleanup()
			// Disable parallel tests until we can resolve this error on the Windows builds:
			// "remote repository at https://github.com/carolynvs/deptest-importers does not exist, or is inaccessible"
			//h.Parallel()

			ctx := importertest.NewTestContext(h)
			sm, err := ctx.SourceManager()
			h.Must(err)
			defer sm.Release()

			i := NewImporter(ctx.Err, ctx.Verbose, sm)
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
		importertest.TestCase
		projects []ImportedPackage
	}{
		"tag constraints are ignored": {
			importertest.TestCase{
				WantConstraint: "*",
				WantVersion:    importertest.Beta1Tag,
				WantRevision:   importertest.Beta1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.Beta1Rev,
					ConstraintHint: importertest.Beta1Tag,
				},
			},
		},
		"tag lock hints Lock to tagged revision": {
			importertest.TestCase{
				WantConstraint: "*",
				WantVersion:    importertest.Beta1Tag,
				WantRevision:   importertest.Beta1Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.Beta1Tag,
				},
			},
		},
		"untagged revision ignores range constraint": {
			importertest.TestCase{
				WantConstraint: "*",
				WantRevision:   importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: importertest.V1Constraint,
				},
			},
		},
		"untagged revision keeps branch constraint": {
			importertest.TestCase{
				WantConstraint: "master",
				WantVersion:    "master",
				WantRevision:   importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: "master",
				},
			},
		},
		"HEAD revisions default constraint to the matching branch": {
			importertest.TestCase{
				DefaultConstraintFromLock: true,
				WantConstraint:            importertest.V2Branch,
				WantVersion:               importertest.V2Branch,
				WantRevision:              importertest.V2Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.V2Rev,
				},
			},
		},
		"Semver tagged revisions default to ^VERSION": {
			importertest.TestCase{
				DefaultConstraintFromLock: true,
				WantConstraint:            importertest.V1Constraint,
				WantVersion:               importertest.V1Tag,
				WantRevision:              importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.V1Rev,
				},
			},
		},
		"Semver lock hint defaults constraint to ^VERSION": {
			importertest.TestCase{
				DefaultConstraintFromLock: true,
				WantConstraint:            importertest.V1Constraint,
				WantVersion:               importertest.V1Tag,
				WantRevision:              importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: importertest.V1Tag,
				},
			},
		},
		"Semver constraint hint": {
			importertest.TestCase{
				WantConstraint: importertest.V1Constraint,
				WantVersion:    importertest.V1PatchTag,
				WantRevision:   importertest.V1PatchRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1PatchRev,
					ConstraintHint: importertest.V1Constraint,
				},
			},
		},
		"Semver prerelease lock hint": {
			importertest.TestCase{
				WantConstraint: importertest.V2Branch,
				WantVersion:    importertest.V2PatchTag,
				WantRevision:   importertest.V2PatchRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V2PatchRev,
					ConstraintHint: importertest.V2Branch,
				},
			},
		},
		"Revision constraints are ignored": {
			importertest.TestCase{
				WantConstraint: "*",
				WantVersion:    importertest.V1Tag,
				WantRevision:   importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1Rev,
					ConstraintHint: importertest.V1Rev,
				},
			},
		},
		"Branch constraint hint": {
			importertest.TestCase{
				WantConstraint: "master",
				WantVersion:    importertest.V1Tag,
				WantRevision:   importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1Rev,
					ConstraintHint: "master",
				},
			},
		},
		"Non-matching semver constraint is ignored": {
			importertest.TestCase{
				WantConstraint: "*",
				WantVersion:    importertest.V1Tag,
				WantRevision:   importertest.V1Rev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.V1Rev,
					ConstraintHint: "^2.0.0",
				},
			},
		},
		"git describe constraint is ignored": {
			importertest.TestCase{
				WantConstraint: "*",
				WantRevision:   importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project,
					LockHint:       importertest.UntaggedRev,
					ConstraintHint: importertest.UntaggedRevAbbrv,
				},
			},
		},
		"consolidate subpackages under root": {
			importertest.TestCase{
				WantConstraint: "master",
				WantVersion:    "master",
				WantRevision:   importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:           importertest.Project + "/subpkA",
					ConstraintHint: "master",
				},
				{
					Name:     importertest.Project,
					LockHint: importertest.UntaggedRev,
				},
			},
		},
		"ignore duplicate packages": {
			importertest.TestCase{
				WantConstraint: "*",
				WantRevision:   importertest.UntaggedRev,
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project + "/subpkgA",
					LockHint: importertest.UntaggedRev, // first wins
				},
				{
					Name:     importertest.Project + "/subpkgB",
					LockHint: importertest.V1Rev,
				},
			},
		},
		"skip empty lock hints": {
			importertest.TestCase{
				WantConstraint: "*",
				WantRevision:   "",
			},
			[]ImportedPackage{
				{
					Name:     importertest.Project,
					LockHint: "",
				},
			},
		},
		"alternate source": {
			importertest.TestCase{
				WantConstraint: "*",
				WantSourceRepo: importertest.ProjectSrc,
			},
			[]ImportedPackage{
				{
					Name:   importertest.Project,
					Source: importertest.ProjectSrc,
				},
			},
		},
		"ignoring default source": {
			importertest.TestCase{
				WantConstraint: "*",
				WantSourceRepo: "",
			},
			[]ImportedPackage{
				{
					Name:   importertest.Project,
					Source: "https://" + importertest.Project,
				},
			},
		},
	}

	for name, tc := range testcases {
		name := name
		tc := tc
		t.Run(name, func(t *testing.T) {
			err := tc.Execute(t, func(logger *log.Logger, sm gps.SourceManager) (*dep.Manifest, *dep.Lock, error) {
				i := NewImporter(logger, true, sm)
				convertErr := i.ImportPackages(tc.projects, tc.DefaultConstraintFromLock)
				return i.Manifest, i.Lock, convertErr
			})
			if err != nil {
				t.Fatalf("%#v", err)
			}
		})
	}
}
