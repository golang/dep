// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/sdboyer/gps"
)

func TestContains(t *testing.T) {
	a := []string{"a", "b", "abcd"}

	if !contains(a, "a") {
		t.Fatal("expected array to contain 'a'")
	}
	if contains(a, "d") {
		t.Fatal("expected array to not contain 'd'")
	}
}

func TestIsStdLib(t *testing.T) {
	tests := map[string]bool{
		"github.com/Sirupsen/logrus": false,
		"encoding/json":              true,
		"golang.org/x/net/context":   false,
		"net/context":                true,
		".":                          false,
	}

	for p, e := range tests {
		b := isStdLib(p)
		if b != e {
			t.Fatalf("%s: expected %t got %t", p, e, b)
		}
	}
}

func TestGetProjectPropertiesFromVersion(t *testing.T) {
	cases := []struct {
		version  gps.Version
		expected gps.Version
	}{
		{
			version:  gps.NewBranch("foo-branch").Is("some-revision"),
			expected: gps.NewBranch("foo-branch"),
		},
		{
			version:  gps.NewVersion("foo-version").Is("some-revision"),
			expected: gps.NewVersion("foo-version"),
		},
		{
			version:  gps.Revision("alsjd934"),
			expected: nil,
		},
		// This fails. Hence, testing separately below.
		// {
		// 	version: gps.NewVersion("v1.0.0"),
		// 	expected: gps.NewVersion("^1.0.0"),
		// },
	}

	for _, c := range cases {
		actualProp := getProjectPropertiesFromVersion(c.version)
		if c.expected != actualProp.Constraint {
			t.Fatalf("Expected %q to be equal to %q", actualProp.Constraint, c.expected)
		}
	}

	outsemver := getProjectPropertiesFromVersion(gps.NewVersion("v1.0.0"))
	expectedSemver, _ := gps.NewSemverConstraint("^1.0.0")
	// Comparing outsemver.Constraint and expectedSemver fails with error
	// "comparing uncomparable type semver.rangeConstraint", although they have
	// same value and same type "gps.semverConstraint" as per "reflect".
	if outsemver.Constraint.String() != expectedSemver.String() {
		t.Fatalf("Expected %q to be equal to %q", outsemver, expectedSemver)
	}
}
