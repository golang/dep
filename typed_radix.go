package gps

import "github.com/armon/go-radix"

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
