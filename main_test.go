// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindRoot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	expect := filepath.Join(wd, "_testdata", "rootfind")

	got1, err := findProjectRoot(expect)
	if err != nil {
		t.Errorf("Unexpected error while finding root: %s", err)
	} else if expect != got1 {
		t.Errorf("findProjectRoot directly on root dir should have found %s, got %s", expect, got1)
	}

	got2, err := findProjectRoot(filepath.Join(expect, "subdir"))
	if err != nil {
		t.Errorf("Unexpected error while finding root: %s", err)
	} else if expect != got2 {
		t.Errorf("findProjectRoot on subdir should have found %s, got %s", expect, got2)
	}

	got3, err := findProjectRoot(filepath.Join(expect, "nonexistent"))
	if err != nil {
		t.Errorf("Unexpected error while finding root: %s", err)
	} else if expect != got3 {
		t.Errorf("findProjectRoot on nonexistent subdir should still work and give %s, got %s", expect, got3)
	}

	// the following test does not work on windows because syscall.Stat does not
	// return a "not a directory" error
	if runtime.GOOS != "windows" {
		got4, err := findProjectRoot(filepath.Join(expect, manifestName))
		if err == nil {
			t.Errorf("Should have err'd when trying subdir of file, but returned %s", got4)
		}
	}
}
