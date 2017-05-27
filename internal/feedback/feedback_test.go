// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feedback

import (
	"testing"
)

func TestGetConstraintString(t *testing.T) {
	cases := []struct {
		feedback string
		want     string
	}{
		{
			feedback: GetUsingFeedback("^1.0.0", ConsTypeConstraint, DepTypeDirect, "github.com/foo/bar"),
			want:     "Using ^1.0.0 as constraint for direct dep github.com/foo/bar",
		},
		{
			feedback: GetUsingFeedback("^1.0.0", ConsTypeConstraint, DepTypeImported, "github.com/foo/bar"),
			want:     "Using ^1.0.0 as initial constraint for imported dep github.com/foo/bar",
		},
		{
			feedback: GetUsingFeedback("1b8edb3", ConsTypeHint, DepTypeDirect, "github.com/bar/baz"),
			want:     "Using 1b8edb3 as hint for direct dep github.com/bar/baz",
		},
		{
			feedback: GetUsingFeedback("1b8edb3", ConsTypeHint, DepTypeImported, "github.com/bar/baz"),
			want:     "Using 1b8edb3 as initial hint for imported dep github.com/bar/baz",
		},
		{
			feedback: GetLockingFeedback("v1.1.4", "bc29b4f", DepTypeDirect, "github.com/foo/bar"),
			want:     "Locking in v1.1.4 (bc29b4f) for direct dep github.com/foo/bar",
		},
		{
			feedback: GetLockingFeedback("v1.1.4", "bc29b4f", DepTypeImported, "github.com/foo/bar"),
			want:     "Trying v1.1.4 (bc29b4f) as initial lock for imported dep github.com/foo/bar",
		},
		{
			feedback: GetLockingFeedback("master", "436f39d", DepTypeTransitive, "github.com/baz/qux"),
			want:     "Locking in master (436f39d) for transitive dep github.com/baz/qux",
		},
	}

	for _, c := range cases {
		if c.want != c.feedback {
			t.Errorf("Feedbacks are not expected: \n\t(GOT) %v\n\t(WNT) %v", c.feedback, c.want)
		}
	}
}
