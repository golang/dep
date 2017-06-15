// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"github.com/golang/dep/internal/gps/pkgtree"
)

func FindIneffectualConstraints(manifest Manifest, packageTree pkgtree.PackageTree) []ProjectRoot {
	var stdLibFn func(string) bool
	// will track return value here
	ineffectuals := make([]ProjectRoot, 0, 10)

	// flatten list of actual imports which should be checked against
	reachmap, _ := packageTree.ToReachMap(true, true, false, nil /*ignores*/)
	reach := reachmap.FlattenFn(stdLibFn)
	imports := make(map[string]bool)
	for _, imp := range reach {
		imports[imp] = true
	}

	// if manifest is a RootManifest, use requires and ignores
	if rootManifest := manifest.(RootManifest); rootManifest != nil {
		// add required packages
		for projectRoot, _ := range rootManifest.RequiredPackages() {
			imports[projectRoot] = true
		}

		// check that ignores actually refer to packages we're importing
		for ignore, _ := range rootManifest.IgnoredPackages() {
			if _, found := imports[ignore]; !found {
				ineffectuals = append(ineffectuals, ProjectRoot(ignore))
			}
		}

		// TODO: Should we remove ignores from the list? Ignores should not be
		// processed, but are constrained ignores ineffectual?
	}

	// at this point we have complete list of imports to test against

	// gather all constraints which should be checked
	constraints := make(map[ProjectRoot]bool) // it's a set to avoid duplicates
	for projectRoot, _ := range manifest.DependencyConstraints() {
		constraints[projectRoot] = true
	}
	for projectRoot, _ := range manifest.TestDependencyConstraints() {
		constraints[projectRoot] = true
	}

	// now check the constraints against the packageTree
	for projectRoot, _ := range constraints {
		if _, used := imports[string(projectRoot)]; !used {
			ineffectuals = append(ineffectuals, projectRoot)
		}
	}

	if len(ineffectuals) > 0 {
		return ineffectuals
	} else {
		return nil
	}
}
