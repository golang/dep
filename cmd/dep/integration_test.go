// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang/dep/test"
)

func TestIntegration(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	filepath.Walk(filepath.Join("testdata", "harness_tests"), func(path string, info os.FileInfo, err error) error {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		if runtime.GOOS == "windows" && strings.Contains(path, "remove") {
			// TODO skipping the remove tests on windows until some fixes happen in gps -
			// see https://github.com/golang/dep/issues/301
			return filepath.SkipDir
		}

		if filepath.Base(path) == "testcase.json" {
			parse := strings.Split(path, string(filepath.Separator))
			testName := strings.Join(parse[2:len(parse)-1], "/")

			t.Run(testName, func(t *testing.T) {
				// Uncomment once the gps improvements are in place
				// t.Parallel()

				// Set up environment
				testCase := test.NewTestCase(t, testName, wd)
				defer testCase.Cleanup()
				testProj := test.NewTestProject(t, testCase.InitialPath(), wd)
				defer testProj.Cleanup()

				// Create and checkout the vendor revisions
				for ip, rev := range testCase.VendorInitial {
					testProj.GetVendorGit(ip)
					testProj.RunGit(testProj.VendorPath(ip), "checkout", rev)
				}

				// Create and checkout the import revisions
				for ip, rev := range testCase.GopathInitial {
					testProj.RunGo("get", ip)
					testProj.RunGit(testProj.Path("src", ip), "checkout", rev)
				}

				// Run commands
				testProj.RecordImportPaths()
				for _, args := range testCase.Commands {
					testProj.DoRun(args)
				}

				// Check final manifest and lock
				testCase.CompareFile("manifest.json", testProj.ProjPath("manifest.json"))
				testCase.CompareFile("lock.json", testProj.ProjPath("lock.json"))

				// Check vendor paths
				testProj.CompareImportPaths()
				testCase.CompareVendorPaths(testProj.GetVendorPaths())
			})
		}
		return nil
	})
}
