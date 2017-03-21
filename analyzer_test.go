// Copyright 2016 The Go Authors. All rights reserved.
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

func TestDeriveManifestAndLock(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("dep")
	golden := "analyzer/manifest.toml"
	want := h.GetTestFileString(golden)
	h.TempCopy(filepath.Join("dep", ManifestName), golden)

	a := analyzer{}

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

func TestDeriveManifestAndLockDoesNotExist(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	a := analyzer{}

	m, l, err := a.DeriveManifestAndLock(dir, "my/fake/project")
	if m != nil || l != nil || err != nil {
		t.Fatalf("expected manifest & lock & err to be nil: m -> %#v l -> %#v err-> %#v", m, l, err)
	}
}
