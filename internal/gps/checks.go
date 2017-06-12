// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"github.com/golang/dep/internal/gps/pkgtree"
)

func FindIneffectualConstraints(manifest Manifest, packageTree pkgtree.PackageTree) []ProjectRoot {
	// gather all constraints which should be checked
	constraints := make(map[ProjectRoot]bool)
	for projectRoot, _ := range manifest.DependencyConstraints() {
		constraints[projectRoot] = true
	}
	for projectRoot, _ := range manifest.TestDependencyConstraints() {
		constraints[projectRoot] = true
	}
	// if manifest is a RootManifest, check requires and ignores
	if rootManifest := manifest.(RootManifest); rootManifest != nil {
		for projectRoot, _ := range rootManifest.RequiredPackages() {
			constraints[ProjectRoot(projectRoot)] = true
		}
		for projectRoot, _ := range rootManifest.IgnoredPackages() {
			constraints[ProjectRoot(projectRoot)] = true
		}
	}

	// now check the constraints against the packageTree
	ineffectuals := make([]ProjectRoot, 0, 10)
	for projectRoot, _ := range constraints {
		if _, used := packageTree.Packages[string(projectRoot)]; !used {
			ineffectuals = append(ineffectuals, projectRoot)
		}
	}

	return ineffectuals
}
