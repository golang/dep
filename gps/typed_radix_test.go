package gps

import "testing"

// basically a regression test
func TestPathPrefixOrEqual(t *testing.T) {
	if !isPathPrefixOrEqual("foo", "foo") {
		t.Error("Same path should return true")
	}

	if isPathPrefixOrEqual("foo", "fooer") {
		t.Error("foo is not a path-type prefix of fooer")
	}

	if !isPathPrefixOrEqual("foo", "foo/bar") {
		t.Error("foo is a path prefix of foo/bar")
	}

	if isPathPrefixOrEqual("foo", "foo/") {
		t.Error("special case - foo is not a path prefix of foo/")
	}
}
