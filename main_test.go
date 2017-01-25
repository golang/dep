// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sdboyer/gps"
)

func TestFindRoot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(wd, "_testdata", "rootfind")
	got1, err := findProjectRoot(want)
	if err != nil {
		t.Errorf("Unexpected error while finding root: %s", err)
	} else if want != got1 {
		t.Errorf("findProjectRoot directly on root dir should have found %s, got %s", want, got1)
	}

	got2, err := findProjectRoot(filepath.Join(want, "subdir"))
	if err != nil {
		t.Errorf("Unexpected error while finding root: %s", err)
	} else if want != got2 {
		t.Errorf("findProjectRoot on subdir should have found %s, got %s", want, got2)
	}

	got3, err := findProjectRoot(filepath.Join(want, "nonexistent"))
	if err != nil {
		t.Errorf("Unexpected error while finding root: %s", err)
	} else if want != got3 {
		t.Errorf("findProjectRoot on nonexistent subdir should still work and give %s, got %s", want, got3)
	}

	root := "/"
	p, err := findProjectRoot(root)
	if p != "" {
		t.Errorf("findProjectRoot with path %s returned non empty string: %s", root, p)
	}
	if err != errProjectNotFound {
		t.Errorf("findProjectRoot want: %#v got: %#v", errProjectNotFound, err)
	}

	// The following test does not work on windows because syscall.Stat does not
	// return a "not a directory" error.
	if runtime.GOOS != "windows" {
		got4, err := findProjectRoot(filepath.Join(want, manifestName))
		if err == nil {
			t.Errorf("Should have err'd when trying subdir of file, but returned %s", got4)
		}
	}
}

func TestProjectMakeParams(t *testing.T) {
	p := project{
		absroot:    "someroot",
		importroot: gps.ProjectRoot("Some project root"),
		m:          &manifest{Ignores: []string{"ignoring this"}},
		l:          &lock{},
	}

	solveParam := p.makeParams()

	if solveParam.Manifest != p.m {
		t.Error("makeParams() returned gps.SolveParameters with incorrect Manifest")
	}

	if solveParam.Lock != p.l {
		t.Error("makeParams() returned gps.SolveParameters with incorrect Lock")
	}
}

func TestSlashedGOPATH(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.tempDir("src")

	tg.setenv("GOPATH", filepath.ToSlash(tg.path(".")))
	_, err := newContext()
	if err != nil {
		t.Fatal(err)
	}

	tg.setenv("GOPATH", filepath.FromSlash(tg.path(".")))
	_, err = newContext()
	if err != nil {
		t.Fatal(err)
	}
}
