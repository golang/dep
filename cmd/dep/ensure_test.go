// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
)

func TestDeduceConstraint(t *testing.T) {
	t.Parallel()
	h := test.NewHelper(t)
	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	sm, err := gps.NewSourceManager(h.Path(cacheDir))
	h.Must(err)

	sv, err := gps.NewSemverConstraintIC("v0.8.1")
	if err != nil {
		t.Fatal(err)
	}

	constraints := map[string]gps.Constraint{
		"v0.8.1": sv,
		"master": gps.NewBranch("master"),
		"5b3352dc16517996fb951394bcbbe913a2a616e3": gps.Revision("5b3352dc16517996fb951394bcbbe913a2a616e3"),

		// valid bzr rev
		"jess@linux.com-20161116211307-wiuilyamo9ian0m7": gps.Revision("jess@linux.com-20161116211307-wiuilyamo9ian0m7"),
		// invalid bzr rev
		"go4@golang.org-sadfasdf-": gps.NewVersion("go4@golang.org-sadfasdf-"),
	}

	pi := gps.ProjectIdentifier{ProjectRoot: "github.com/sdboyer/deptest"}
	for str, want := range constraints {
		got, err := deduceConstraint(str, pi, sm)
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

func TestDeduceConstraint_InvalidInput(t *testing.T) {
	h := test.NewHelper(t)

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	sm, err := gps.NewSourceManager(h.Path(cacheDir))
	h.Must(err)

	constraints := []string{
		// invalid bzr revs
		"go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn",
		"20120425195858-psty8c35ve2oej8t",
	}

	pi := gps.ProjectIdentifier{ProjectRoot: "github.com/sdboyer/deptest"}
	for _, str := range constraints {
		_, err := deduceConstraint(str, pi, sm)
		if err == nil {
			t.Errorf("expected %s to produce an error", str)
		}
	}
}
