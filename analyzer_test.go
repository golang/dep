// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang/dep/test"
)

func TestAnalyzerDeriveManifestAndLock(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("dep")
	golden := filepath.Join("analyzer", ManifestName)
	want := h.GetTestFileString(golden)
	h.TempCopy(filepath.Join("dep", ManifestName), golden)

	a := Analyzer{}

	m, l, err := a.DeriveManifestAndLock(h.Path("dep"), "my/fake/project")
	if err != nil {
		t.Fatal(err)
	}

	got, err := m.(*Manifest).MarshalTOML()
	if err != nil {
		t.Fatal(err)
	}

	if want != string(got) {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(golden, string(got)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s\n got %s", want, string(got))
		}
	}

	if l != nil {
		t.Fatalf("expected lock to be nil, got: %#v", l)
	}
}

func TestAnalyzerDeriveManifestAndLockDoesNotExist(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("dep")

	a := Analyzer{}

	m, l, err := a.DeriveManifestAndLock(h.Path("dep"), "my/fake/project")
	if m != nil || l != nil || err != nil {
		t.Fatalf("expected manifest & lock & err to be nil: m -> %#v l -> %#v err-> %#v", m, l, err)
	}
}

func TestAnalyzerDeriveManifestAndLockCannotOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		// TODO: find an implementation that works on Microsoft
		// Windows. Setting permissions works differently there.
		// os.Chmod(..., 0222) below is not enough for the file
		// to be write-only (unreadable), and os.Chmod(...,
		// 0000) returns an invalid argument error.
		t.Skip("skipping on windows")
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("dep")

	// Create an empty manifest file
	h.TempFile(filepath.Join("dep", ManifestName), "")

	// Change its mode so that it cannot be read
	err := os.Chmod(filepath.Join(h.Path("dep"), ManifestName), 0222)
	if err != nil {
		t.Fatal(err)
	}

	a := Analyzer{}

	m, l, err := a.DeriveManifestAndLock(h.Path("dep"), "my/fake/project")
	if m != nil || l != nil || err == nil {
		t.Fatalf("expected manifest & lock to be nil, err to be not nil: m -> %#v l -> %#v err -> %#v", m, l, err)
	}
}

func TestAnalyzerDeriveManifestAndLockInvalidManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("dep")

	// Create a manifest with invalid contents
	h.TempFile(filepath.Join("dep", ManifestName), "invalid manifest")

	a := Analyzer{}

	m, l, err := a.DeriveManifestAndLock(h.Path("dep"), "my/fake/project")
	if m != nil || l != nil || err == nil {
		t.Fatalf("expected manifest & lock & err to be nil: m -> %#v l -> %#v err-> %#v", m, l, err)
	}
}

func TestAnalyzerInfo(t *testing.T) {
	a := Analyzer{}

	name, vers := a.Info()

	if name != "dep" || vers != 1 {
		t.Fatalf("expected name to be 'dep' and version to be 1: name -> %q vers -> %d", name, vers)
	}
}
