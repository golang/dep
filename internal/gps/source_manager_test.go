// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"reflect"
	"testing"

	"github.com/golang/dep/internal/test"
)

func TestSourceManager_InferConstraint(t *testing.T) {
	t.Parallel()

	testcase := func(str string, pi ProjectIdentifier, want Constraint) func(*testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			h := test.NewHelper(t)
			defer h.Cleanup()

			cacheDir := "gps-repocache"
			h.TempDir(cacheDir)

			sm, err := NewSourceManager(h.Path(cacheDir))
			h.Must(err)

			got, err := sm.InferConstraint(str, pi)
			h.Must(err)

			wantT := reflect.TypeOf(want)
			gotT := reflect.TypeOf(got)
			if wantT != gotT {
				t.Errorf("expected type: %s, got %s, for input %s", wantT, gotT, str)
			}
			if got.String() != want.String() {
				t.Errorf("expected value: %s, got %s for input %s", want, got, str)
			}
		}
	}

	var (
		gitProj = ProjectIdentifier{ProjectRoot: "github.com/carolynvs/deptest"}
		bzrProj = ProjectIdentifier{ProjectRoot: "launchpad.net/govcstestbzrrepo"}
		hgProj  = ProjectIdentifier{ProjectRoot: "bitbucket.org/golang-dep/dep-test"}
	)

	t.Run("git", func(t *testing.T) {
		t.Parallel()
		t.Run("empty", testcase("", gitProj, Any()))

		v081, err := NewSemverConstraintIC("v0.8.1")
		if err != nil {
			t.Fatal(err)
		}

		v012, err := NewSemverConstraintIC("v0.12.0-12-de4dcafe0")
		if err != nil {
			t.Fatal(err)
		}

		t.Run("semver constraint", testcase("v0.8.1", gitProj, v081))
		t.Run("long semver constraint", testcase("v0.12.0-12-de4dcafe0", gitProj, v012))
		t.Run("branch v2", testcase("v2", gitProj, NewBranch("v2")))
		t.Run("branch master", testcase("master", gitProj, NewBranch("master")))
		t.Run("long revision", testcase("3f4c3bea144e112a69bbe5d8d01c1b09a544253f", gitProj, Revision("3f4c3bea144e112a69bbe5d8d01c1b09a544253f")))
		t.Run("short revision", testcase("3f4c3bea", gitProj, Revision("3f4c3bea144e112a69bbe5d8d01c1b09a544253f")))
	})

	t.Run("bzr", func(t *testing.T) {
		t.Parallel()
		v1, err := NewSemverConstraintIC("v1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		t.Run("empty", testcase("", bzrProj, Any()))
		t.Run("semver", testcase("v1.0.0", bzrProj, v1))
		t.Run("revision", testcase("matt@mattfarina.com-20150731135137-pbphasfppmygpl68", bzrProj, Revision("matt@mattfarina.com-20150731135137-pbphasfppmygpl68")))
	})

	t.Run("hg", func(t *testing.T) {
		t.Parallel()
		v1, err := NewSemverConstraintIC("v1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		t.Run("empty", testcase("", hgProj, Any()))
		t.Run("semver", testcase("v1.0.0", hgProj, v1))
		t.Run("default branch", testcase("default", hgProj, NewBranch("default")))
		t.Run("revision", testcase("6f55e1f03d91f8a7cce35d1968eb60a2352e4d59", hgProj, Revision("6f55e1f03d91f8a7cce35d1968eb60a2352e4d59")))
		t.Run("short revision", testcase("6f55e1f03d91", hgProj, Revision("6f55e1f03d91f8a7cce35d1968eb60a2352e4d59")))
	})
}
