// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

// PruneOptions represents the pruning options used to write the dependecy tree.
type PruneOptions uint8

const (
	// PruneNestedVendorDirs indicates if nested vendor directories should be pruned.
	PruneNestedVendorDirs = 1 << iota
	// PruneUnusedPackages indicates if unused Go packages should be pruned.
	PruneUnusedPackages
	// PruneNonGoFiles indicates if non-Go files should be pruned.
	// LICENSE & COPYING files are kept for convience.
	PruneNonGoFiles
	// PruneGoTestFiles indicates if Go test files should be pruned.
	PruneGoTestFiles
)

var (
	preservedNonGoFiles = []string{
		"LICENSE",
		"COPYING",
	}
)
