// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/test"
)

func TestEmptyProject(t *testing.T) {
	g := new(graphviz).New()
	h := test.NewHelper(t)
	defer h.Cleanup()

	b := g.output()
	expected := h.GetTestFileString("graphviz/empty.dot")

	if b.String() != expected {
		t.Fatalf("expected '%v', got '%v'", expected, b.String())
	}
}

func TestSimpleProject(t *testing.T) {
	g := new(graphviz).New()
	h := test.NewHelper(t)
	defer h.Cleanup()

	g.createNode("project", "", []string{"foo", "bar"})
	g.createNode("foo", "master", []string{"bar"})
	g.createNode("bar", "dev", []string{})

	b := g.output()
	expected := h.GetTestFileString("graphviz/case1.dot")
	if b.String() != expected {
		t.Fatalf("expected '%v', got '%v'", expected, b.String())
	}
}

func TestNoLinks(t *testing.T) {
	g := new(graphviz).New()
	h := test.NewHelper(t)
	defer h.Cleanup()

	g.createNode("project", "", []string{})

	b := g.output()
	expected := h.GetTestFileString("graphviz/case2.dot")
	if b.String() != expected {
		t.Fatalf("expected '%v', got '%v'", expected, b.String())
	}
}

func TestIsPathPrefix(t *testing.T) {
	tcs := []struct {
		path     string
		pre      string
		expected bool
	}{
		{"github.com/sdboyer/foo/bar", "github.com/sdboyer/foo", true},
		{"github.com/sdboyer/foobar", "github.com/sdboyer/foo", false},
		{"github.com/sdboyer/bar/foo", "github.com/sdboyer/foo", false},
		{"golang.org/sdboyer/bar/foo", "github.com/sdboyer/foo", false},
		{"golang.org/sdboyer/FOO", "github.com/sdboyer/foo", false},
	}

	for _, tc := range tcs {
		r := isPathPrefix(tc.path, tc.pre)
		if tc.expected != r {
			t.Fatalf("expected '%v', got '%v'", tc.expected, r)
		}
	}
}
