// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/dep/test"
)

func TestIntegration(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, "commands.txt") {
			parse := strings.Split(path, string(filepath.Separator))
			testName := strings.Join(parse[1:len(parse)-1], "/")

			t.Run(testName, func(t *testing.T) {
				// Set up environment
				testCase := test.NewTestCase(t, testName)
				testProj := test.NewTestProject(t, testCase.InitialPath)
				defer testProj.Cleanup()

				// Checkout the specified revisions
				importPaths := testCase.GetImports()
				for ip, rev := range importPaths {
					testProj.RunGo("get", ip)
					testProj.RunGit(testProj.Path("src", ip), "checkout", rev)
				}

				// Run commands
				commands := testCase.GetCommands()
				for _, args := range commands {
					testProj.DoRun(args)
				}

				// Check final manifest and lock
				testCase.CompareFile("manifest.json", testProj.ProjPath("manifest.json"))
				testCase.CompareFile("lock.json", testProj.ProjPath("lock.json"))

				// Check vendor paths
				testCase.CompareVendorPaths(testProj.GetVendorPaths())
			})
		}
		return nil
	})
}
