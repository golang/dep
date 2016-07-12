// +build ignore

package main

import (
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strings"

	gps "github.com/sdboyer/vsolver"
)

func main() {
	// Operate on the current directory
	root, _ := os.Getwd()
	// Assume the current directory is correctly placed on a GOPATH, and derive
	// the ProjectRoot from it
	importroot := strings.TrimPrefix(root, filepath.Join(build.Default.GOPATH, "src")+string(filepath.Separator))

	params := gps.SolveParameters{
		RootDir:     root,
		ImportRoot:  gps.ProjectRoot(importroot),
		Trace:       true,
		TraceLogger: log.New(os.Stdout, "", 0),
	}

	sourcemgr, _ := gps.NewSourceManager(MyAnalyzer{}, ".repocache", false)
	defer sourcemgr.Release()

	solver, _ := gps.Prepare(params, sourcemgr)
	solution, err := solver.Solve()
	if err == nil {
		os.RemoveAll(filepath.Join(root, "vendor"))
		gps.CreateVendorTree(filepath.Join(root, "vendor"), solution, sourcemgr, true)
	}
}

type MyAnalyzer struct{}

func (a MyAnalyzer) GetInfo(path string, n gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	return nil, nil, nil
}
