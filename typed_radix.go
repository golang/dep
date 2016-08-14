package gps

import (
	"strings"

	"github.com/armon/go-radix"
)

// Typed implementations of radix trees. These are just simple wrappers that let
// us avoid having to type assert anywhere else, cleaning up other code a bit.
//
// Some of the more annoying things to implement (like walks) aren't
// implemented. They can be added if/when we actually need them.
//
// Oh generics, where art thou...

type deducerTrie struct {
	t *radix.Tree
}

func newDeducerTrie() deducerTrie {
	return deducerTrie{
		t: radix.New(),
	}
}

// Delete is used to delete a key, returning the previous value and if it was deleted
func (t deducerTrie) Delete(s string) (pathDeducer, bool) {
	if v, had := t.t.Delete(s); had {
		return v.(pathDeducer), had
	}
	return nil, false
}

// Get is used to lookup a specific key, returning the value and if it was found
func (t deducerTrie) Get(s string) (pathDeducer, bool) {
	if v, has := t.t.Get(s); has {
		return v.(pathDeducer), has
	}
	return nil, false
}

// Insert is used to add a newentry or update an existing entry. Returns if updated.
func (t deducerTrie) Insert(s string, v pathDeducer) (pathDeducer, bool) {
	if v2, had := t.t.Insert(s, v); had {
		return v2.(pathDeducer), had
	}
	return nil, false
}

// Len is used to return the number of elements in the tree
func (t deducerTrie) Len() int {
	return t.t.Len()
}

// LongestPrefix is like Get, but instead of an exact match, it will return the
// longest prefix match.
func (t deducerTrie) LongestPrefix(s string) (string, pathDeducer, bool) {
	if p, v, has := t.t.LongestPrefix(s); has {
		return p, v.(pathDeducer), has
	}
	return "", nil, false
}

// ToMap is used to walk the tree and convert it to a map.
func (t deducerTrie) ToMap() map[string]pathDeducer {
	m := make(map[string]pathDeducer)
	t.t.Walk(func(s string, v interface{}) bool {
		m[s] = v.(pathDeducer)
		return false
	})

	return m
}

type prTrie struct {
	t *radix.Tree
}

func newProjectRootTrie() prTrie {
	return prTrie{
		t: radix.New(),
	}
}

// Delete is used to delete a key, returning the previous value and if it was deleted
func (t prTrie) Delete(s string) (ProjectRoot, bool) {
	if v, had := t.t.Delete(s); had {
		return v.(ProjectRoot), had
	}
	return "", false
}

// Get is used to lookup a specific key, returning the value and if it was found
func (t prTrie) Get(s string) (ProjectRoot, bool) {
	if v, has := t.t.Get(s); has {
		return v.(ProjectRoot), has
	}
	return "", false
}

// Insert is used to add a newentry or update an existing entry. Returns if updated.
func (t prTrie) Insert(s string, v ProjectRoot) (ProjectRoot, bool) {
	if v2, had := t.t.Insert(s, v); had {
		return v2.(ProjectRoot), had
	}
	return "", false
}

// Len is used to return the number of elements in the tree
func (t prTrie) Len() int {
	return t.t.Len()
}

// LongestPrefix is like Get, but instead of an exact match, it will return the
// longest prefix match.
func (t prTrie) LongestPrefix(s string) (string, ProjectRoot, bool) {
	if p, v, has := t.t.LongestPrefix(s); has && isPathPrefixOrEqual(p, s) {
		return p, v.(ProjectRoot), has
	}
	return "", "", false
}

// ToMap is used to walk the tree and convert it to a map.
func (t prTrie) ToMap() map[string]ProjectRoot {
	m := make(map[string]ProjectRoot)
	t.t.Walk(func(s string, v interface{}) bool {
		m[s] = v.(ProjectRoot)
		return false
	})

	return m
}

// isPathPrefixOrEqual is an additional helper check to ensure that the literal
// string prefix returned from a radix tree prefix match is also a tree match.
//
// The radix tree gets it mostly right, but we have to guard against
// possibilities like this:
//
// github.com/sdboyer/foo
// github.com/sdboyer/foobar/baz
//
// The latter would incorrectly be conflated with the former. As we know we're
// operating on strings that describe paths, guard against this case by
// verifying that either the input is the same length as the match (in which
// case we know they're equal), or that the next character is a "/".
func isPathPrefixOrEqual(pre, path string) bool {
	prflen := len(pre)
	return prflen == len(path) || strings.Index(path[:prflen], "/") == 0
}
