// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
)

// rootProjectAnalyzer is responsible for generating a manifest and lock for
// a root project.
type rootProjectAnalyzer interface {
	// Generate an initial manifest and lock for the root project.
	DeriveRootManifestAndLock(path string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error)

	// Apply any final changes to the manifest and lock after the solver has been run.
	FinalizeManifestAndLock(*dep.Manifest, *dep.Lock)
}
