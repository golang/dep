// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"

	"github.com/golang/dep/internal/gps"
)

func TestDeduceConstraint(t *testing.T) {
	t.Parallel()

	sv, err := gps.NewSemverConstraintIC("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	constraints := map[string]gps.Constraint{
		"v1.2.3": sv,
		"5b3352dc16517996fb951394bcbbe913a2a616e3": gps.Revision("5b3352dc16517996fb951394bcbbe913a2a616e3"),

		// valid bzr revs
		"jess@linux.com-20161116211307-wiuilyamo9ian0m7": gps.Revision("jess@linux.com-20161116211307-wiuilyamo9ian0m7"),

		// invalid bzr revs
		"go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn": gps.NewVersion("go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn"),
		"go4@golang.org-sadfasdf-":                  gps.NewVersion("go4@golang.org-sadfasdf-"),
		"20120425195858-psty8c35ve2oej8t":           gps.NewVersion("20120425195858-psty8c35ve2oej8t"),
	}

	for str, want := range constraints {
		got := deduceConstraint(str)

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
