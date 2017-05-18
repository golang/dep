// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package paths

import "strings"

// IsStandardImportPath indicates whether the provided path should be considered
// part of the standard library by checking the first path element for a '.'.
//
// This was lovingly lifted from src/cmd/go/pkg.go in Go's code.
func IsStandardImportPath(path string) bool {
	i := strings.Index(path, "/")
	if i < 0 {
		i = len(path)
	}

	return !strings.Contains(path[:i], ".")
}
