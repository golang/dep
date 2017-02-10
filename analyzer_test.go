// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/golang/dep/test"
)

func TestAnalyzerInfo(t *testing.T) {
	a := analyzer{}
	n, v := a.Info()
	if n != "dep" {
		t.Errorf("analyzer.Info() returned an incorrect name: '%s' (expected 'dep')", n)
	}
	expV, err := semver.NewVersion("v0.0.1")
	if err != nil {
		t.Fatal(err)
	} else if v != expV {
		t.Fatalf("analyzer.Info() returned an incorrect version: %v (expected %v)", v, expV)
	}
}

func TestAnalyzerErrors(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	h.TempDir("dep")

	a := analyzer{}
	_, _, err := a.DeriveManifestAndLock(h.Path("dep"), "my/fake/project")
	if err == nil {
		t.Fatal("analyzer.DeriveManifestAndLock with manifest not present should have produced an error")
	}
}

func TestDeriveManifestAndLock(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("dep")
	golden := "analyzer/manifest.json"
	contents := h.GetTestFileString(golden)
	h.TempCopy(filepath.Join("dep", ManifestName), golden)

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
		if *test.UpdateGolden {
			if err := h.WriteTestFile(golden, string(b)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s\n got %s", contents, string(b))
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
