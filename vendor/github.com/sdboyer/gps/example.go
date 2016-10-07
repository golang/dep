// +build ignore

package main

import (
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/sdboyer/gps"
)

// This is probably the simplest possible implementation of gps. It does the
// substantive work that `go get` does, except:
//  1. It drops the resulting tree into vendor instead of GOPATH
//  2. It prefers semver tags (if available) over branches
//  3. It removes any vendor directories nested within dependencies
//
//  This will compile and work...and then blow away any vendor directory present
//  in the cwd. Be careful!
func main() {
	// Operate on the current directory
	root, _ := os.Getwd()
	// Assume the current directory is correctly placed on a GOPATH, and derive
	// the ProjectRoot from it
	srcprefix := filepath.Join(build.Default.GOPATH, "src") + string(filepath.Separator)
	importroot := filepath.ToSlash(strings.TrimPrefix(root, srcprefix))

	// Set up params, including tracing
	params := gps.SolveParameters{
		RootDir:     root,
		Trace:       true,
		TraceLogger: log.New(os.Stdout, "", 0),
	}
	params.RootPackageTree, _ = gps.ListPackages(root, importroot)

	// Set up a SourceManager with the NaiveAnalyzer
	sourcemgr, _ := gps.NewSourceManager(NaiveAnalyzer{}, ".repocache")
	defer sourcemgr.Release()

	// Prep and run the solver
	solver, _ := gps.Prepare(params, sourcemgr)
	solution, err := solver.Solve()
	if err == nil {
		// If no failure, blow away the vendor dir and write a new one out,
		// stripping nested vendor directories as we go.
		os.RemoveAll(filepath.Join(root, "vendor"))
		gps.WriteDepTree(filepath.Join(root, "vendor"), solution, sourcemgr, true)
	}
}

type NaiveAnalyzer struct{}

// DeriveManifestAndLock gets called when the solver needs manifest/lock data
// for a particular project (the gps.ProjectRoot parameter) at a particular
// version. That version will be checked out in a directory rooted at path.
func (a NaiveAnalyzer) DeriveManifestAndLock(path string, n gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	return nil, nil, nil
}

// Reports the name and version of the analyzer. This is mostly irrelevant.
func (a NaiveAnalyzer) Info() (name string, version *semver.Version) {
	v, _ := semver.NewVersion("v0.0.1")
	return "example-analyzer", v
}
