// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
)

// rootProjectAnalyzer is responsible for generating a root manifest and lock for
// a root project.
type rootProjectAnalyzer interface {
	// Perform analysis of the filesystem tree rooted at path, with the
	// root import path importRoot, to determine the project's constraints, as
	// indicated by a Manifest and Lock.
	DeriveRootManifestAndLock(path string, n gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error)

	PostSolveShenanigans(*dep.Manifest, *dep.Lock)
}
