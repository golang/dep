// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/test"
)

func TestFindRoot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(wd, "testdata", "rootfind")
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
		got4, err := findProjectRoot(filepath.Join(want, ManifestName))
		if err == nil {
			t.Errorf("Should have err'd when trying subdir of file, but returned %s", got4)
		}
	}
}

func TestProjectMakeParams(t *testing.T) {
	p := Project{
		AbsRoot:    "someroot",
		ImportRoot: gps.ProjectRoot("Some project root"),
		Manifest:   &Manifest{Ignored: []string{"ignoring this"}},
		Lock:       &Lock{},
	}

	solveParam := p.MakeParams()

	if solveParam.Manifest != p.Manifest {
		t.Error("makeParams() returned gps.SolveParameters with incorrect Manifest")
	}

	if solveParam.Lock != p.Lock {
		t.Error("makeParams() returned gps.SolveParameters with incorrect Lock")
	}
}

func TestSlashedGOPATH(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	h.TempDir("src")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal("failed to get work directory:", err)
	}
	env := os.Environ()

	h.Setenv("GOPATH", filepath.ToSlash(h.Path(".")))
	_, err = NewContext(wd, env)
	if err != nil {
		t.Fatal(err)
	}

	h.Setenv("GOPATH", filepath.FromSlash(h.Path(".")))
	_, err = NewContext(wd, env)
	if err != nil {
		t.Fatal(err)
	}
}
