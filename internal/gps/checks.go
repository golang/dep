// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"github.com/golang/dep/internal/gps/pkgtree"
)

func FindIneffectualConstraints(manifest Manifest, packageTree pkgtree.PackageTree) ProjectConstraints {
	// Merge the normal and test constraints together
	constraints := manifest.DependencyConstraints().merge(manifest.TestDependencyConstraints())

	// var workingConstraints []workingConstraint
	// var ignoredPackages map[string]bool
	// var requiredPackages map[string]bool
	// if manifest is a RootManifest, then apply the overrides
	if rootManifest := manifest.(RootManifest); rootManifest != nil {
		// Include any overrides that are not already in the constraint list.
		// Doing so makes input hashes equal in more useful cases.
		for projectRoot, projectProperties := range rootManifest.Overrides() {
			if _, exists := constraints[projectRoot]; !exists {
				pp := ProjectProperties{
					Constraint: projectProperties.Constraint,
					Source:     projectProperties.Source,
				}
				if pp.Constraint == nil {
					pp.Constraint = anyConstraint{}
				}

				constraints[projectRoot] = pp
			}
		}

		// ignoredPackages = rootManifest.IgnoredPackages()
		// requiredPackages = rootManifest.RequiredPackages()
	}

	var ineffectuals ProjectConstraints
	for packageRoot, packageProperties := range constraints {
		if _, used := packageTree.Packages[string(packageRoot)]; !used {
			ineffectuals[packageRoot] = packageProperties
		}
	}

	return ineffectuals
}

// // externalImportList returns a list of the unique imports from the root data.
// // Ignores and requires are taken into consideration, stdlib is excluded, and
// // errors within the local set of package are not backpropagated.
// func externalImportList2(packageTree pkgtree.PackageTree, requiredPackages, ignoredPackages map[string]bool, stdLibFn func(string) bool) []string {
// 	reachMap, _ := packageTree.ToReachMap(true, true, false, ignoredPackages)
// 	reach := reachMap.FlattenFn(stdLibFn)
//
// 	// If there are any requires, slide them into the reach list, as well.
// 	if len(requiredPackages) > 0 {
// 		// Make a map of imports that are both in the import path list and the
// 		// required list to avoid duplication.
// 		skip := make(map[string]bool, len(requiredPackages))
// 		for _, r := range reach {
// 			if requiredPackages[r] {
// 				skip[r] = true
// 			}
// 		}
//
// 		for r := range requiredPackages {
// 			if !skip[r] {
// 				reach = append(reach, r)
// 			}
// 		}
// 	}
//
// 	sort.Strings(reach)
// 	return reach
// }
