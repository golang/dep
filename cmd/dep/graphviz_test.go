// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/internal/test"
)

func TestEmptyProject(t *testing.T) {
	h := test.NewHelper(t)
	h.Parallel()
	defer h.Cleanup()

	g := new(graphviz).New()

	b := g.output()
	want := h.GetTestFileString("graphviz/empty.dot")

	if b.String() != want {
		t.Fatalf("expected '%v', got '%v'", want, b.String())
	}
}

func TestSimpleProject(t *testing.T) {
	h := test.NewHelper(t)
	h.Parallel()
	defer h.Cleanup()

	g := new(graphviz).New()

	g.createNode("project", "", []string{"foo", "bar"})
	g.createNode("foo", "master", []string{"bar"})
	g.createNode("bar", "dev", []string{})

	b := g.output()
	want := h.GetTestFileString("graphviz/case1.dot")
	if b.String() != want {
		t.Fatalf("expected '%v', got '%v'", want, b.String())
	}
}

func TestNoLinks(t *testing.T) {
	h := test.NewHelper(t)
	h.Parallel()
	defer h.Cleanup()

	g := new(graphviz).New()

	g.createNode("project", "", []string{})

	b := g.output()
	want := h.GetTestFileString("graphviz/case2.dot")
	if b.String() != want {
		t.Fatalf("expected '%v', got '%v'", want, b.String())
	}
}

func TestIsPathPrefix(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		path string
		pre  string
		want bool
	}{
		{"github.com/sdboyer/foo/bar", "github.com/sdboyer/foo", true},
		{"github.com/sdboyer/foobar", "github.com/sdboyer/foo", false},
		{"github.com/sdboyer/bar/foo", "github.com/sdboyer/foo", false},
		{"golang.org/sdboyer/bar/foo", "github.com/sdboyer/foo", false},
		{"golang.org/sdboyer/FOO", "github.com/sdboyer/foo", false},
	}

	for _, tc := range tcs {
		r := isPathPrefix(tc.path, tc.pre)
		if tc.want != r {
			t.Fatalf("expected '%v', got '%v'", tc.want, r)
		}
	}
}
