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

// MockIsStdLib sets the IsStdLib func to always return false, otherwise it would identify
// pretty much all of our fixtures as being stdlib and skip everything.
//
// The function is not designed to be used from anywhere else except gps's fixtures initialization.
func MockIsStdLib() {
	IsStdLib = func(path string) bool {
		return false
	}
}
