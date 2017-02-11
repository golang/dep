// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveManifestAndLock(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	contents := `{
    "dependencies": {
        "github.com/pkg/errors": {
            "version": ">=0.8.0, <1.0.0"
        },
        "github.com/sdboyer/gps": {
            "version": ">=0.12.0, <1.0.0"
        }
    }
}
`

	if err := ioutil.WriteFile(filepath.Join(dir, ManifestName), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	a := analyzer{}

	m, l, err := a.DeriveManifestAndLock(dir, "my/fake/project")
	if err != nil {
		t.Fatal(err)
	}

	b := new(bytes.Buffer)

	_, err = m.(*Manifest).WriteTo(b)
	if err != nil {
		t.Fatal(err)
	}

	if (b.String()) != contents {
		t.Fatalf("expected %s\n got %s", contents, b.String())
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
