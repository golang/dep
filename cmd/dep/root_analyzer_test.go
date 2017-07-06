// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
)

func TestRootAnalyzer_Info(t *testing.T) {
	testCases := map[bool]string{
		true:  "dep",
		false: "dep+import",
	}
	for skipTools, want := range testCases {
		a := rootAnalyzer{skipTools: skipTools}
		got := a.Info().Name
		if got != want {
			t.Errorf("Expected the name of the importer with skipTools=%t to be '%s', got '%s'", skipTools, want, got)
		}
	}
}

func TestLookupVersionForLockedProject_MatchRevisionToTag(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	rev := gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")
	v, err := lookupVersionForLockedProject(pi, nil, rev, sm)
	h.Must(err)

	wantV := "v1.0.0"
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to be the tag paired with the manifest's pinned revision: wanted '%s', got '%s'", wantV, gotV)
	}
}

func TestLookupVersionForLockedProject_MatchRevisionToMultipleTags(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	// Both 0.8.0 and 1.0.0 use the same rev, force dep to pick the lower version
	c, _ := gps.NewSemverConstraint("<1.0.0")
	rev := gps.Revision("ff2948a2ac8f538c4ecd55962e919d1e13e74baf")
	v, err := lookupVersionForLockedProject(pi, c, rev, sm)
	h.Must(err)

	wantV := "v0.8.0"
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to satisfy the manifest's semver constraint: wanted '%s', got '%s'", wantV, gotV)
	}
}

func TestLookupVersionForLockedProject_FallbackToConstraint(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	c := gps.NewBranch("master")
	rev := gps.Revision("c575196502940c07bf89fd6d95e83b999162e051")
	v, err := lookupVersionForLockedProject(pi, c, rev, sm)
	h.Must(err)

	wantV := c.String()
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to be defaulted from the manifest's branch constraint: wanted '%s', got '%s'", wantV, gotV)
	}
}

func TestLookupVersionForLockedProject_FallbackToRevision(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := newTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/deptest")}
	rev := gps.Revision("c575196502940c07bf89fd6d95e83b999162e051")
	v, err := lookupVersionForLockedProject(pi, nil, rev, sm)
	h.Must(err)

	wantV := rev.String()
	gotV := v.String()
	if gotV != wantV {
		t.Fatalf("Expected the locked version to be the manifest's pinned revision: wanted '%s', got '%s'", wantV, gotV)
	}
}
