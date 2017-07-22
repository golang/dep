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
	h := test.NewHelper(t)
	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	sm, err := NewSourceManager(h.Path(cacheDir))
	h.Must(err)

	sv, err := NewSemverConstraintIC("v0.8.1")
	if err != nil {
		t.Fatal(err)
	}

	svs, err := NewSemverConstraintIC("v0.12.0-12-de4dcafe0")
	if err != nil {
		t.Fatal(err)
	}

	constraints := map[string]Constraint{
		"v0.8.1": sv,
		"v2":     NewBranch("v2"),
		"v0.12.0-12-de4dcafe0": svs,
		"master":               NewBranch("master"),
		"5b3352dc16517996fb951394bcbbe913a2a616e3": Revision("5b3352dc16517996fb951394bcbbe913a2a616e3"),

		// valid bzr rev
		"jess@linux.com-20161116211307-wiuilyamo9ian0m7": Revision("jess@linux.com-20161116211307-wiuilyamo9ian0m7"),
		// invalid bzr rev
		"go4@golang.org-sadfasdf-": NewVersion("go4@golang.org-sadfasdf-"),
	}

	pi := ProjectIdentifier{ProjectRoot: "github.com/carolynvs/deptest"}
	for str, want := range constraints {
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

func TestSourceManager_InferConstraint_InvalidInput(t *testing.T) {
	h := test.NewHelper(t)

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	sm, err := NewSourceManager(h.Path(cacheDir))
	h.Must(err)

	constraints := []string{
		// invalid bzr revs
		"go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn",
		"20120425195858-psty8c35ve2oej8t",
	}

	pi := ProjectIdentifier{ProjectRoot: "github.com/sdboyer/deptest"}
	for _, str := range constraints {
		_, err := sm.InferConstraint(str, pi)
		if err == nil {
			t.Errorf("expected %s to produce an error", str)
		}
	}
}
