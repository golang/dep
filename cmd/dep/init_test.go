// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/test"
)

func TestContains(t *testing.T) {
	a := []string{"a", "b", "abcd"}

	if !contains(a, "a") {
		t.Fatal("expected array to contain 'a'")
	}
	if contains(a, "d") {
		t.Fatal("expected array to not contain 'd'")
	}
}

func TestIsStdLib(t *testing.T) {
	tests := map[string]bool{
		"github.com/Sirupsen/logrus": false,
		"encoding/json":              true,
		"golang.org/x/net/context":   false,
		"net/context":                true,
		".":                          false,
	}

	for p, e := range tests {
		b := isStdLib(p)
		if b != e {
			t.Fatalf("%s: expected %t got %t", p, e, b)
		}
	}
}

func TestInit(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))

	importPaths := map[string]string{
		"github.com/pkg/errors":      "v0.8.0",                                   // semver
		"github.com/Sirupsen/logrus": "42b84f9ec624953ecbf81a94feccb3f5935c5edf", // random sha
	}

	// checkout the specified revisions
	for ip, rev := range importPaths {
		h.RunGo("get", ip)
		repoDir := h.Path("src/" + ip)
		h.RunGit(repoDir, "checkout", rev)
	}

	// Build a fake consumer of these packages.
	const root = "notexist.com/something.git"
	m := `package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"

	"` + root + `/foo/bar"
)

func main() {
	err := nil
	if err != nil {
		errors.Wrap(err, "thing")
	}
	logrus.Info(bar.Qux)
}`

	h.TempFile("src/"+root+"/foo/thing.go", m)

	m = `package bar

const Qux = "yo yo!"
`
	h.TempFile("src/"+root+"/foo/bar/bar.go", m)

	h.Cd(h.Path("src/" + root))
	h.Run("init")

	expectedManifest := `{
    "dependencies": {
        "github.com/Sirupsen/logrus": {
            "revision": "42b84f9ec624953ecbf81a94feccb3f5935c5edf"
        },
        "github.com/pkg/errors": {
            "version": ">=0.8.0, <1.0.0"
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
    "memo": "668fe45796bc4e85a5a6c0a0f1eb6fab9e23588d1eb33f3a12b2ad5599a3575e",
    "projects": [
        {
            "name": "github.com/Sirupsen/logrus",
            "revision": "42b84f9ec624953ecbf81a94feccb3f5935c5edf",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/pkg/errors",
            "version": "v0.8.0",
            "revision": "645ef00459ed84a119197bfb8d8205042c6df63d",
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

	// vendor should have been created & populated
	h.MustExist(h.Path("src/" + root + "/vendor/github.com/Sirupsen/logrus"))
}
