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

	tests := []struct {
		name       string
		want, werr bool
	}{
		{name: wd, want: false, werr: true},
		{name: filepath.Join(wd, "testdata"), want: false, werr: true},
		{name: filepath.Join(wd, "cmd", "dep", "main.go"), want: true, werr: false},
		{name: filepath.Join(wd, "this_file_does_not_exist.thing"), want: false, werr: false},
	}

	for _, test := range tests {
		got, err := IsRegular(test.name)
		if test.werr && err == nil {
			t.Fatalf("wanted an error for %q, but it was nil", test.name)
		}
		if !test.werr && err != nil {
			t.Fatalf("did not want an error for %q, but instead got: %s", test.name, err)
		}
		if test.want != got {
			t.Fatalf("wanted %t for %q, but instead got: %t", test.want, test.name, got)
		}
	}
}

func TestIsDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		want, werr bool
	}{
		{name: wd, want: true, werr: false},
		{name: filepath.Join(wd, "testdata"), want: true, werr: false},
		{name: filepath.Join(wd, "main.go"), want: false, werr: false},
		{name: filepath.Join(wd, "this_file_does_not_exist.thing"), want: false, werr: false},
	}

	for _, test := range tests {
		got, err := IsDir(test.name)
		if test.werr && err == nil {
			t.Fatalf("wanted an error for %q, but it was nil", test.name)
		}
		if !test.werr && err != nil {
			t.Fatalf("did not want an error for %q, but instead got: %s", test.name, err)
		}
		if test.want != got {
			t.Fatalf("wanted %t for %q, but instead got: %t", test.want, test.name, got)
		}
	}
}

func TestIsEmptyDirOrNotExist(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	existingFile := filepath.Join(wd, "fs.go")
	doesNotExist := filepath.Join(wd, "this_file_does_not_exist.thing")

	h := test.NewHelper(t)
	h.TempDir("empty")
	tests := []struct {
		name       string
		want, werr bool
	}{
		{name: wd, want: false, werr: false},             // not empty, exists
		{name: "testdata", want: false, werr: false},     // not empty, exists
		{name: existingFile, want: false, werr: true},    // exists, but file
		{name: doesNotExist, want: true, werr: false},    // does not exist
		{name: h.Path("empty"), want: true, werr: false}, // empty, exists
	}
	for _, test := range tests {
		got, err := IsEmptyDirOrNotExist(test.name)
		if test.werr && err == nil {
			t.Fatalf("wanted an error for %q, but it was nil", test.name)
		}
		if !test.werr && err != nil {
			t.Fatalf("did not want an error for %q, but instead got: %s", test.name, err)
		}
		if test.want != got {
			t.Fatalf("wanted %t for %q, but instead got: %t", test.want, test.name, got)
		}
	}
}
