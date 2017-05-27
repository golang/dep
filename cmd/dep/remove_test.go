// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/dep/internal/test"
)

func TestRemoveErrors(t *testing.T) {
	t.Parallel()

	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	testName := "remove/unused/case1"
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	t.Run(testName+"/external", removeErrors(testName, wd, true, execCmd))
	t.Run(testName+"/internal", removeErrors(testName, wd, false, runMain))
}

func removeErrors(name, wd string, externalProc bool, run test.RunFunc) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		testCase := test.NewTestCase(t, filepath.Join(wd, "testdata", "harness_tests"), name)
		testProj := test.NewTestProject(t, testCase.InitialPath(), wd, externalProc, run)
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

		if err := testProj.DoRun([]string{"remove", "-unused", "github.com/not/used"}); err == nil {
			t.Fatal("rm with both -unused and arg should have failed")
		}

		if err := testProj.DoRun([]string{"remove", "github.com/not/present"}); err == nil {
			t.Fatal("rm with arg not in manifest should have failed")
		}

		if err := testProj.DoRun([]string{"remove", "github.com/not/used", "github.com/not/present"}); err == nil {
			t.Fatal("rm with one arg not in manifest should have failed")
		}

		if err := testProj.DoRun([]string{"remove", "github.com/sdboyer/deptest"}); err == nil {
			t.Fatal("rm of arg in manifest and imports should have failed without -force")
		}
	}
}
