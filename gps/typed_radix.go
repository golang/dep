package gps

import (
	"strings"
	"sync"

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
	sync.RWMutex
	t *radix.Tree
}

func newDeducerTrie() *deducerTrie {
	return &deducerTrie{
		t: radix.New(),
	}
}

// Delete is used to delete a key, returning the previous value and if it was deleted
func (t *deducerTrie) Delete(s string) (pathDeducer, bool) {
	t.Lock()
	defer t.Unlock()
	if d, had := t.t.Delete(s); had {
		return d.(pathDeducer), had
	}
	return nil, false
}

// Get is used to lookup a specific key, returning the value and if it was found
func (t *deducerTrie) Get(s string) (pathDeducer, bool) {
	t.RLock()
	defer t.RUnlock()
	if d, has := t.t.Get(s); has {
		return d.(pathDeducer), has
	}
	return nil, false
}

// Insert is used to add a newentry or update an existing entry. Returns if updated.
func (t *deducerTrie) Insert(s string, d pathDeducer) (pathDeducer, bool) {
	t.Lock()
	defer t.Unlock()
	if d2, had := t.t.Insert(s, d); had {
		return d2.(pathDeducer), had
	}
	return nil, false
}

// Len is used to return the number of elements in the tree
func (t *deducerTrie) Len() int {
	t.RLock()
	defer t.RUnlock()
	return t.t.Len()
}

// LongestPrefix is like Get, but instead of an exact match, it will return the
// longest prefix match.
func (t *deducerTrie) LongestPrefix(s string) (string, pathDeducer, bool) {
	t.RLock()
	defer t.RUnlock()
	if p, d, has := t.t.LongestPrefix(s); has {
		return p, d.(pathDeducer), has
	}
	return "", nil, false
}

// ToMap is used to walk the tree and convert it to a map.
func (t *deducerTrie) ToMap() map[string]pathDeducer {
	m := make(map[string]pathDeducer)
	t.RLock()
	t.t.Walk(func(s string, d interface{}) bool {
		m[s] = d.(pathDeducer)
		return false
	})

	t.RUnlock()
	return m
}

// isPathPrefixOrEqual is an additional helper check to ensure that the literal
// string prefix returned from a radix tree prefix match is also a path tree
// match.
//
// The radix tree gets it mostly right, but we have to guard against
// possibilities like this:
//
// github.com/sdboyer/foo
// github.com/sdboyer/foobar/baz
//
// The latter would incorrectly be conflated with the former. As we know we're
// operating on strings that describe import paths, guard against this case by
// verifying that either the input is the same length as the match (in which
// case we know they're equal), or that the next character is a "/". (Import
// paths are defined to always use "/", not the OS-specific path separator.)
func isPathPrefixOrEqual(pre, path string) bool {
	prflen, pathlen := len(pre), len(path)
	if pathlen == prflen+1 {
		// this can never be the case
		return false
	}

	// we assume something else (a trie) has done equality check up to the point
	// of the prefix, so we just check len
	return prflen == pathlen || strings.Index(path[prflen:], "/") == 0
}
