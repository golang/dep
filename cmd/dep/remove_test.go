// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/test"
)

func TestRemoveErrors(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	testName := "remove/unused/case1"

	testCase := test.NewTestCase(t, testName)
	testProj := test.NewTestProject(t, testCase.InitialPath)
	defer testProj.Cleanup()

	// Create and checkout the vendor revisions
	vendorPaths := testCase.GetInitVendors()
	for ip, rev := range vendorPaths {
		testProj.GetVendorGit(ip)
		testProj.RunGit(testProj.VendorPath(ip), "checkout", rev)
	}

	// Create and checkout the import revisions
	importPaths := testCase.GetImports()
	for ip, rev := range importPaths {
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
