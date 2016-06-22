// +build go1.6

package vsolver

import "go/build"

// analysisImportMode returns the import mode used for build.Import() calls for
// standard package analysis.
func analysisImportMode() build.ImportMode {
	return build.ImportComment | build.IgnoreVendor
}
