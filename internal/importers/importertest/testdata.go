// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package importertest

const (
	// RootProject is the containing project performing the import.
	RootProject = "github.com/golang/notexist"

	// Project being imported.
	Project = "github.com/carolynvs/deptest-importers"

	// ProjectSrc is an alternate source for the imported project.
	ProjectSrc = "https://github.com/carolynvs/deptest-importers.git"

	// UntaggedRev is a revision without any tags.
	UntaggedRev = "9b670d143bfb4a00f7461451d5c4a62f80e9d11d"

	// UntaggedRevAbbrv is the result of running `git describe` on UntaggedRev
	UntaggedRevAbbrv = "v1.0.0-1-g9b670d1"

	// Beta1Tag is a non-semver tag.
	Beta1Tag = "beta1"

	// Beta1Rev is the revision of Beta1Tag
	Beta1Rev = "7913ab26988c6fb1e16225f845a178e8849dd254"

	// V2Branch is a branch that could be interpreted as a semver tag (but shouldn't).
	V2Branch = "v2"

	// V2Rev is the HEAD revision of V2Branch.
	V2Rev = "45dcf5a09c64b48b6e836028a3bc672b19b9d11d"

	// V2PatchTag is a prerelease semver tag on the non-default branch.
	V2PatchTag = "v2.0.0-alpha1"

	// V2PatchRev is the revision of V2PatchTag.
	V2PatchRev = "347760b50204948ea63e531dd6560e56a9adde8f"

	// V1Tag is a semver tag that matches V1Constraint.
	V1Tag = "v1.0.0"

	// V1Rev is the revision of V1Tag.
	V1Rev = "d0c29640b17f77426b111f4c1640d716591aa70e"

	// V1PatchTag is a semver tag that matches V1Constraint.
	V1PatchTag = "v1.0.2"

	// V1PatchRev is the revision of V1PatchTag
	V1PatchRev = "788963efe22e3e6e24c776a11a57468bb2fcd780"

	// V1Constraint is a constraint that matches multiple semver tags.
	V1Constraint = "^1.0.0"

	// MultiTaggedRev is a revision with multiple tags.
	MultiTaggedRev = "34cf993cc346f65601fe4356dd68bd54d20a1bfe"

	// MultiTaggedSemverTag is a semver tag on MultiTaggedRev.
	MultiTaggedSemverTag = "v1.0.4"

	// MultiTaggedPlainTag is a non-semver tag on MultiTaggedRev.
	MultiTaggedPlainTag = "stable"

	// NonexistentPrj is a dummy project which does not exist on Github.
	NonexistentPrj = "github.com/nonexistent/project"
)
