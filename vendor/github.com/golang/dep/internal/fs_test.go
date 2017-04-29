// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestHasFilepathPrefix(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cases := []struct {
		dir    string
		prefix string
		want   bool
	}{
		{filepath.Join(dir, "a", "b"), filepath.Join(dir), true},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir, "a"), true},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir, "a", "b"), true},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir, "c"), false},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir, "a", "d", "b"), false},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir, "a", "b2"), false},
		{filepath.Join(dir, "ab"), filepath.Join(dir, "a", "b"), false},
		{filepath.Join(dir, "ab"), filepath.Join(dir, "a"), false},
		{filepath.Join(dir, "123"), filepath.Join(dir, "123"), true},
		{filepath.Join(dir, "123"), filepath.Join(dir, "1"), false},
		{filepath.Join(dir, "⌘"), filepath.Join(dir, "⌘"), true},
		{filepath.Join(dir, "a"), filepath.Join(dir, "⌘"), false},
		{filepath.Join(dir, "⌘"), filepath.Join(dir, "a"), false},
	}

	for _, c := range cases {
		err := os.MkdirAll(c.dir, 0755)
		if err != nil {
			t.Fatal(err)
		}

		err = os.MkdirAll(c.prefix, 0755)
		if err != nil {
			t.Fatal(err)
		}

		got := HasFilepathPrefix(c.dir, c.prefix)
		if c.want != got {
			t.Fatalf("dir: %q, prefix: %q, expected: %v, got: %v", c.dir, c.prefix, c.want, got)
		}
	}
}

func TestGenTestFilename(t *testing.T) {
	cases := []struct {
		str  string
		want string
	}{
		{"abc", "Abc"},
		{"ABC", "aBC"},
		{"AbC", "abC"},
		{"αβγ", "Αβγ"},
		{"123", "123"},
		{"1a2", "1A2"},
		{"12a", "12A"},
		{"⌘", "⌘"},
	}

	for _, c := range cases {
		got := genTestFilename(c.str)
		if c.want != got {
			t.Fatalf("str: %q, expected: %q, got: %q", c.str, c.want, got)
		}
	}
}

func BenchmarkGenTestFilename(b *testing.B) {
	cases := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"αααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααα",
		"11111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111",
		"⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘",
	}

	for i := 0; i < b.N; i++ {
		for _, str := range cases {
			genTestFilename(str)
		}
	}
}
