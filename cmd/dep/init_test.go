// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"

	"github.com/golang/dep/internal/gps"
)

func TestContains(t *testing.T) {
	t.Parallel()

	a := []string{"a", "b", "abcd"}

	if !contains(a, "a") {
		t.Fatal("expected array to contain 'a'")
	}
	if contains(a, "d") {
		t.Fatal("expected array to not contain 'd'")
	}
}

func TestGetProjectPropertiesFromVersion(t *testing.T) {
	t.Parallel()

	wantSemver, _ := gps.NewSemverConstraintIC("v1.0.0")
	cases := []struct {
		version, want gps.Constraint
	}{
		{
			version: gps.NewBranch("foo-branch"),
			want:    gps.NewBranch("foo-branch"),
		},
		{
			version: gps.NewVersion("foo-version"),
			want:    gps.NewVersion("foo-version"),
		},
		{
			version: gps.NewVersion("v1.0.0"),
			want:    wantSemver,
		},
		{
			version: gps.NewBranch("foo-branch").Is("some-revision"),
			want:    gps.NewBranch("foo-branch"),
		},
		{
			version: gps.NewVersion("foo-version").Is("some-revision"),
			want:    gps.NewVersion("foo-version"),
		},
		{
			version: gps.Revision("some-revision"),
			want:    nil,
		},
		{
			version: gps.NewVersion("v1.0.0").Is("some-revision"),
			want:    wantSemver,
		},
	}

	for _, c := range cases {
		actualProp := getProjectPropertiesFromVersion(c.version.(gps.Version))
		if !reflect.DeepEqual(c.want, actualProp.Constraint) {
			t.Fatalf("Constraints are not as expected: \n\t(GOT) %v\n\t(WNT) %v", actualProp.Constraint, c.want)
		}
	}
}
