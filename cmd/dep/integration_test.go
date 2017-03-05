// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
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
					err := testProj.DoRun(args)
					fmt.Println(args, err)
				}

				// Check final manifest and lock
				testCase.CompareFile(t, "manifest.json", testProj.ProjPath("manifest.json"))
				testCase.CompareFile(t, "lock.json", testProj.ProjPath("lock.json"))

				// Check vendor paths
				wantVendorPaths := testCase.GetVendors()
				gotVendorPaths := testProj.GetVendorPaths()
				if len(gotVendorPaths) != len(wantVendorPaths) {
					t.Errorf("Wrong number of vendor paths created: want %d got %d", len(gotVendorPaths), len(wantVendorPaths))
				}
				for ind := range gotVendorPaths {
					if gotVendorPaths[ind] != wantVendorPaths[ind] {
						t.Errorf("Mismatch in vendor paths created: want %s got %s", gotVendorPaths, wantVendorPaths)
					}
				}
			})
		}
		return nil
	})
}
