// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal provides support for gps own packages.
package internal

import "strings"

// IsStdLib is a reference to internal implementation.
// It is stored as a var so that tests can swap it out. Ugh globals, ugh.
var IsStdLib = doIsStdLib

// This was lovingly lifted from src/cmd/go/pkg.go in Go's code
// (isStandardImportPath).
func doIsStdLib(path string) bool {
	i := strings.Index(path, "/")
	if i < 0 {
		i = len(path)
	}

	return !strings.Contains(path[:i], ".")
}
