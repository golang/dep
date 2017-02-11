// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/test"
	"github.com/sdboyer/gps"
)

func TestEnsureOverrides(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))

	m := `package main

import (
	"github.com/Sirupsen/logrus"
	sthing "github.com/sdboyer/dep-test"
)

type Baz sthing.Foo

func main() {
	logrus.Info("hello world")
}`

	h.TempFile("src/thing/thing.go", m)
	h.Cd(h.Path("src/thing"))

	h.Run("init")
	h.Run("ensure", "-override", "github.com/Sirupsen/logrus@0.11.0")

	expectedManifest := `{
    "overrides": {
        "github.com/Sirupsen/logrus": {
            "version": "0.11.0"
        }
    }
}
`

	manifest := h.ReadManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit := h.GetCommit("go.googlesource.com/sys")
	expectedLock := `{
    "memo": "57d20ba0289c2df60025bf6127220a5403483251bd5e523a7f9ea17752bd5482",
    "projects": [
        {
            "name": "github.com/Sirupsen/logrus",
            "version": "v0.11.0",
            "revision": "d26492970760ca5d33129d2d799e34be5c4782eb",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/sdboyer/dep-test",
            "version": "1.0.0",
            "revision": "2a3a211e171803acb82d1d5d42ceb53228f51751",
            "packages": [
                "."
            ]
        },
        {
            "name": "golang.org/x/sys",
            "branch": "master",
            "revision": "` + sysCommit + `",
            "packages": [
                "unix"
            ]
        }
    ]
}
`
	lock := h.ReadLock()
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}

func TestEnsureEmptyRepoNoArgs(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))

	m := `package main

import (
	_ "github.com/jimmysmith95/fixed-version"
	_ "golang.org/x/sys/unix"
)

func main() {
}`

	h.TempFile("src/thing/thing.go", m)
	h.Cd(h.Path("src/thing"))

	h.Run("init")
	h.Run("ensure")

	// make sure vendor exists
	h.MustExist(h.Path("src/thing/vendor/github.com/jimmysmith95/fixed-version"))

	expectedManifest := `{}
`

	manifest := h.ReadManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit := h.GetCommit("go.googlesource.com/sys")
	fixedVersionCommit := h.GetCommit("github.com/jimmysmith95/fixed-version")

	expectedLock := `{
    "memo": "8a7660015b2473d6d2f4bfdfd0508e6aa8178746559d0a9a90cecfbe6aa47a06",
    "projects": [
        {
            "name": "github.com/jimmysmith95/fixed-version",
            "version": "v1.0.0",
            "revision": "` + fixedVersionCommit + `",
            "packages": [
                "."
            ]
        },
        {
            "name": "golang.org/x/sys",
            "branch": "master",
            "revision": "` + sysCommit + `",
            "packages": [
                "unix"
            ]
        }
    ]
}
`

	lock := h.ReadLock()
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}

func TestDeduceConstraint(t *testing.T) {
	sv, err := gps.NewSemverConstraint("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	constraints := map[string]gps.Constraint{
		"v1.2.3": sv,
		"5b3352dc16517996fb951394bcbbe913a2a616e3": gps.Revision("5b3352dc16517996fb951394bcbbe913a2a616e3"),

		// valid bzr revs
		"jess@linux.com-20161116211307-wiuilyamo9ian0m7": gps.Revision("jess@linux.com-20161116211307-wiuilyamo9ian0m7"),

		// invalid bzr revs
		"go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn": gps.NewVersion("go4@golang.org-lskjdfnkjsdnf-ksjdfnskjdfn"),
		"go4@golang.org-sadfasdf-":                  gps.NewVersion("go4@golang.org-sadfasdf-"),
		"20120425195858-psty8c35ve2oej8t":           gps.NewVersion("20120425195858-psty8c35ve2oej8t"),
	}

	for str, expected := range constraints {
		c := deduceConstraint(str)
		if c != expected {
			t.Fatalf("expected: %#v, got %#v for %s", expected, c, str)
		}
	}
}

func TestEnsureUpdate(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))

	m := `package main

import (
	"fmt"
	"github.com/pkg/errors"
	stuff "github.com/carolynvs/go-dep-test"
)

func main() {
	fmt.Println(stuff.Thing)
	TryToDoSomething()
}

func TryToDoSomething() error {
	return errors.New("I tried, Billy. I tried...")
}
`

	h.TempFile("src/thing/thing.go", m)

	origManifest := `{
    "dependencies": {
        "github.com/carolynvs/go-dep-test": {
            "version": "~0.1.0"
        }
    }
}`
	h.TempFile("src/thing/manifest.json", origManifest)

	origLock := `{
    "memo": "9a5243dd3fa20feeaa20398e7283d6c566532e2af1aae279a010df34793761c5",
    "projects": [
        {
            "name": "github.com/carolynvs/go-dep-test",
            "version": "0.1.0",
            "revision": "b9c5511fa463628e6251554db29a4be161d02aed",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/pkg/errors",
            "branch": "v0.7.0",
            "revision": "01fa4104b9c248c8945d14d9f128454d5b28d595",
            "packages": [
                "."
            ]
        }
    ]
}
`
	h.TempFile("src/thing/lock.json", origLock)
	h.Cd(h.Path("src/thing"))

	h.Run("ensure", "-update", "github.com/carolynvs/go-dep-test")

	manifest := h.ReadManifest()
	if manifest != origManifest {
		t.Fatalf("The manifest should not be modified during an update. Expected %s, got %s", origManifest, manifest)
	}

	expectedLock := `{
    "memo": "9a5243dd3fa20feeaa20398e7283d6c566532e2af1aae279a010df34793761c5",
    "projects": [
        {
            "name": "github.com/carolynvs/go-dep-test",
            "version": "0.1.1",
            "revision": "40691983e4002d3a3f5879cc0f1fe99bedda148c",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/pkg/errors",
            "branch": "v0.7.0",
            "revision": "01fa4104b9c248c8945d14d9f128454d5b28d595",
            "packages": [
                "."
            ]
        }
    ]
}
`
	lock := h.ReadLock()
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}
