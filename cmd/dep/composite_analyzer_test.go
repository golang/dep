// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
)

type testRootProjectAnalyzer struct {
	*dep.Manifest
	*dep.Lock
}

func (a testRootProjectAnalyzer) DeriveRootManifestAndLock(path string, n gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	return a.Manifest, a.Lock, nil
}

func (a testRootProjectAnalyzer) FinalizeManifestAndLock(*dep.Manifest, *dep.Lock) {
	// do nothing
}

func TestCompositeAnalyzer_ManifestDependencies(t *testing.T) {
	pkg := gps.ProjectRoot("github.com/sdboyer/deptest")
	m1 := &dep.Manifest{
		Dependencies: gps.ProjectConstraints{
			pkg: gps.ProjectProperties{Constraint: gps.NewVersion("^1.0.0")},
		},
	}
	m2 := &dep.Manifest{
		Dependencies: gps.ProjectConstraints{
			pkg: gps.ProjectProperties{Constraint: gps.NewVersion("^2.0.0")},
		},
	}
	c := compositeAnalyzer{
		Analyzers: []rootProjectAnalyzer{
			testRootProjectAnalyzer{Manifest: m1},
			testRootProjectAnalyzer{Manifest: m2},
		},
	}

	rm, _, err := c.DeriveRootManifestAndLock("", "")
	if err != nil {
		t.Fatal(err)
	}

	if rm == nil {
		t.Fatal("Expected the root manifest to not be nil")
	}

	dep, has := rm.Dependencies[pkg]
	if !has {
		t.Fatal("Expected the root manifest to contain the test project")
	}

	wantC := "^2.0.0"
	gotC := dep.Constraint.String()
	if wantC != gotC {
		t.Fatalf("Expected the test project to be constrained to '%s', got '%s'", wantC, gotC)
	}
}

func TestCompositeAnalyzer_ManifestOverrides(t *testing.T) {
	pkg := gps.ProjectRoot("github.com/sdboyer/deptest")
	m1 := &dep.Manifest{
		Ovr: gps.ProjectConstraints{
			pkg: gps.ProjectProperties{Constraint: gps.NewVersion("^1.0.0")},
		},
	}
	m2 := &dep.Manifest{
		Ovr: gps.ProjectConstraints{
			pkg: gps.ProjectProperties{Constraint: gps.NewVersion("^2.0.0")},
		},
	}
	c := compositeAnalyzer{
		Analyzers: []rootProjectAnalyzer{
			testRootProjectAnalyzer{Manifest: m1},
			testRootProjectAnalyzer{Manifest: m2},
		},
	}

	rm, _, err := c.DeriveRootManifestAndLock("", "")
	if err != nil {
		t.Fatal(err)
	}

	if rm == nil {
		t.Fatal("Expected the root manifest to not be nil")
	}

	dep, has := rm.Ovr[pkg]
	if !has {
		t.Fatal("Expected the root manifest to contain the test project override")
	}

	wantC := "^2.0.0"
	gotC := dep.Constraint.String()
	if wantC != gotC {
		t.Fatalf("Expected the test project to be overridden to '%s', got '%s'", wantC, gotC)
	}
}

func TestCompositeAnalyzer_ManifestRequired(t *testing.T) {
	pkg1 := "github.com/sdboyer/deptest"
	pkg2 := "github.com/sdboyer/deptestdos"
	m1 := &dep.Manifest{
		Required: []string{pkg1},
	}
	m2 := &dep.Manifest{
		Required: []string{pkg2},
	}
	c := compositeAnalyzer{
		Analyzers: []rootProjectAnalyzer{
			testRootProjectAnalyzer{Manifest: m1},
			testRootProjectAnalyzer{Manifest: m2},
		},
	}

	rm, _, err := c.DeriveRootManifestAndLock("", "")
	if err != nil {
		t.Fatal(err)
	}

	if rm == nil {
		t.Fatal("Expected the root manifest to not be nil")
	}

	if len(rm.Required) != 2 {
		t.Fatalf("Expected the root manifest to contain 2 required packages, got %d", len(rm.Required))
	}

	if rm.Required[0] != pkg1 {
		t.Fatalf("Expected the first required package to be '%s', got '%s'", pkg1, rm.Required[0])
	}

	if rm.Required[1] != pkg2 {
		t.Fatalf("Expected the second required package to be '%s', got '%s'", pkg2, rm.Required[1])
	}
}

func TestCompositeAnalyzer_ManifestIgnored(t *testing.T) {
	pkg1 := "github.com/sdboyer/deptest"
	pkg2 := "github.com/sdboyer/deptestdos"
	m1 := &dep.Manifest{
		Ignored: []string{pkg1},
	}
	m2 := &dep.Manifest{
		Ignored: []string{pkg2},
	}
	c := compositeAnalyzer{
		Analyzers: []rootProjectAnalyzer{
			testRootProjectAnalyzer{Manifest: m1},
			testRootProjectAnalyzer{Manifest: m2},
		},
	}

	rm, _, err := c.DeriveRootManifestAndLock("", "")
	if err != nil {
		t.Fatal(err)
	}

	if rm == nil {
		t.Fatal("Expected the root manifest to not be nil")
	}

	if len(rm.Ignored) != 2 {
		t.Fatalf("Expected the root manifest to contain 2 ignored packages, got %d", len(rm.Ignored))
	}

	if rm.Ignored[0] != pkg1 {
		t.Fatalf("Expected the first ignored package to be '%s', got '%s'", pkg1, rm.Ignored[0])
	}

	if rm.Ignored[1] != pkg2 {
		t.Fatalf("Expected the second ignored package to be '%s', got '%s'", pkg2, rm.Ignored[1])
	}
}
