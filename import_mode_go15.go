// +build !go1.6

package gps

import "go/build"

// analysisImportMode returns the import mode used for build.Import() calls for
// standard package analysis.
//
// build.NoVendor was added in go1.6, so we have to omit it here.
func analysisImportMode() build.ImportMode {
	return build.ImportComment
}
