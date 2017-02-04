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
	contents := h.GetTestFileString("analyzer/manifest.json")
	h.TempCopy(filepath.Join("dep", ManifestName), "analyzer/manifest.json")

	a := analyzer{}

	m, l, err := a.DeriveManifestAndLock(h.Path("dep"), "my/fake/project")
	if err != nil {
		t.Fatal(err)
	}

	b, err := m.(*Manifest).MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	if contents != string(b) {
		t.Fatalf("expected %s\n got %s", contents, string(b))
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
