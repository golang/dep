// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
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

	h.TempCopy("src/thing/thing.go", "ensure_test/source1.go")
	h.Cd(h.Path("src/thing"))

	h.Run("init")
	h.Run("ensure", "-override", "github.com/Sirupsen/logrus@0.11.0")

	expectedManifest := h.GetTestfile("ensure_test/exp_manifest1.json")
	manifest := h.ReadManifest()
	if exp, err := test.AreEqualJSON(expectedManifest, manifest); !exp {
		h.Must(err)
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit := h.GetCommit("go.googlesource.com/sys")
	expectedLock := h.GetTestfile("ensure_test/exp_lock1.json")
	expectedLock = strings.Replace(expectedLock, "{{sysCommit}}", sysCommit, -1)
	lock := h.ReadLock()
	if exp, err := test.AreEqualJSON(expectedLock, lock); !exp {
		h.Must(err)
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
	"github.com/Sirupsen/logrus"
)

func main() {
	logrus.Info("hello world")
}`

	h.TempFile("src/thing/thing.go", m)
	h.Cd(h.Path("src/thing"))

	h.Run("init")
	h.Run("ensure")

	// make sure vendor exists
	h.MustExist(h.Path("src/thing/vendor/github.com/Sirupsen/logrus"))

	expectedManifest := `{}
`

	manifest := h.ReadManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit := h.GetCommit("go.googlesource.com/sys")
	logrusCommit := h.GetCommit("github.com/Sirupsen/logrus")
	expectedLock := `{
    "memo": "d4a7b45d366ece090464407f4038cdb62a031c29ef3254f197b8a3d5e6993cca",
    "projects": [
        {
            "name": "github.com/Sirupsen/logrus",
            "version": "v0.11.0",
            "revision": "` + logrusCommit + `",
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
