// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/dep/test"
)

func TestCopyDir(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcdir := filepath.Join(dir, "src")
	if err = os.MkdirAll(srcdir, 0755); err != nil {
		t.Fatal(err)
	}

	srcf, err := os.Create(filepath.Join(srcdir, "myfile"))
	if err != nil {
		t.Fatal(err)
	}

	want := "hello world"
	if _, err = srcf.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	destdir := filepath.Join(dir, "dest")
	if err = CopyDir(srcdir, destdir); err != nil {
		t.Fatal(err)
	}

	dirOK, err := IsDir(destdir)
	if err != nil {
		t.Fatal(err)
	}
	if !dirOK {
		t.Fatalf("expected %s to be a directory", destdir)
	}

	destf := filepath.Join(destdir, "myfile")
	got, err := ioutil.ReadFile(destf)
	if err != nil {
		t.Fatal(err)
	}

	if want != string(got) {
		t.Fatalf("expected: %s, got: %s", want, string(got))
	}

	wantinfo, err := os.Stat(srcf.Name())
	if err != nil {
		t.Fatal(err)
	}

	gotinfo, err := os.Stat(destf)
	if err != nil {
		t.Fatal(err)
	}

	if wantinfo.Mode() != gotinfo.Mode() {
		t.Fatalf("expected %s: %#v\n to be the same mode as %s: %#v", srcf.Name(), wantinfo.Mode(), destf, gotinfo.Mode())
	}
}

func TestCopyFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcf, err := os.Create(filepath.Join(dir, "srcfile"))
	if err != nil {
		t.Fatal(err)
	}

	want := "hello world"
	if _, err := srcf.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	destf := filepath.Join(dir, "destf")
	if err := CopyFile(srcf.Name(), destf); err != nil {
		t.Fatal(err)
	}

	got, err := ioutil.ReadFile(destf)
	if err != nil {
		t.Fatal(err)
	}

	if want != string(got) {
		t.Fatalf("expected: %s, got: %s", want, string(got))
	}

	wantinfo, err := os.Stat(srcf.Name())
	if err != nil {
		t.Fatal(err)
	}

	gotinfo, err := os.Stat(destf)
	if err != nil {
		t.Fatal(err)
	}

	if wantinfo.Mode() != gotinfo.Mode() {
		t.Fatalf("expected %s: %#v\n to be the same mode as %s: %#v", srcf.Name(), wantinfo.Mode(), destf, gotinfo.Mode())
	}
}

func TestIsRegular(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]bool{
		wd: false,
		filepath.Join(wd, "testdata"):                       false,
		filepath.Join(wd, "cmd", "dep", "main.go"):          true,
		filepath.Join(wd, "this_file_does_not_exist.thing"): false,
	}

	for f, want := range tests {
		got, err := IsRegular(f)
		if err != nil {
			if !want {
				// this is the case where we expect an error so continue
				// to the check below
				continue
			}
			t.Fatalf("expected no error, got %v", err)
		}

		if got != want {
			t.Fatalf("expected %t for %s, got %t", want, f, got)
		}
	}

}

func TestIsDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]bool{
		wd: true,
		filepath.Join(wd, "testdata"):                       true,
		filepath.Join(wd, "main.go"):                        false,
		filepath.Join(wd, "this_file_does_not_exist.thing"): false,
	}

	for f, want := range tests {
		got, err := IsDir(f)
		if err != nil {
			if !want {
				// this is the case where we expect an error so continue
				// to the check below
				continue
			}
			t.Fatalf("expected no error, got %v", err)
		}

		if got != want {
			t.Fatalf("expected %t for %s, got %t", want, f, got)
		}
	}

}

func TestIsEmpty(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	h := test.NewHelper(t)
	h.TempDir("empty")
	tests := map[string]string{
		wd:                                                  "true",
		"testdata":                                          "true",
		filepath.Join(wd, "fs.go"):                          "err",
		filepath.Join(wd, "this_file_does_not_exist.thing"): "false",
		h.Path("empty"):                                     "false",
	}

	for f, want := range tests {
		empty, err := IsNonEmptyDir(f)
		if want == "err" {
			if err == nil {
				t.Fatalf("Wanted an error for %v, but it was nil", f)
			}
			if empty {
				t.Fatalf("Wanted false with error for %v, but got true", f)
			}
		} else if err != nil {
			t.Fatalf("Wanted no error for %v, got %v", f, err)
		}

		if want == "true" && !empty {
			t.Fatalf("Wanted true for %v, but got false", f)
		}

		if want == "false" && empty {
			t.Fatalf("Wanted false for %v, but got true", f)
		}
	}
}
