// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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

func TestIsRegular(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]bool{
		wd: false,
		filepath.Join(wd, "_testdata"):                      false,
		filepath.Join(wd, "main.go"):                        true,
		filepath.Join(wd, "this_file_does_not_exist.thing"): false,
	}

	for f, expected := range tests {
		fileOK, err := isRegular(f)
		if err != nil {
			if !expected {
				// this is the case where we expect an error so continue
				// to the check below
				continue
			}
			t.Fatalf("expected no error, got %v", err)
		}

		if fileOK != expected {
			t.Fatalf("expected %t for %s, got %t", expected, f, fileOK)
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
		filepath.Join(wd, "_testdata"):                      true,
		filepath.Join(wd, "main.go"):                        false,
		filepath.Join(wd, "this_file_does_not_exist.thing"): false,
	}

	for f, expected := range tests {
		dirOK, err := isDir(f)
		if err != nil {
			if !expected {
				// this is the case where we expect an error so continue
				// to the check below
				continue
			}
			t.Fatalf("expected no error, got %v", err)
		}

		if dirOK != expected {
			t.Fatalf("expected %t for %s, got %t", expected, f, dirOK)
		}
	}

}

func TestInit(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)
	// TODO: fix and remove this skip on windows
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows momentarily")
	}

	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))

	importPaths := map[string]string{
		"github.com/pkg/errors":      "v0.8.0",                                   // semver
		"github.com/Sirupsen/logrus": "42b84f9ec624953ecbf81a94feccb3f5935c5edf", // random sha
	}

	// checkout the specified revisions
	for ip, rev := range importPaths {
		tg.runGo("get", ip)
		repoDir := tg.path("src/" + ip)
		tg.runGit(repoDir, "checkout", rev)
	}

	// Build a fake consumer of these packages.
	const root = "github.com/golang/notexist"
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

	tg.tempFile("src/"+root+"/foo/thing.go", m)

	m = `package bar

const Qux = "yo yo!"
`
	tg.tempFile("src/"+root+"/foo/bar/bar.go", m)

	tg.cd(tg.path("src/" + root))
	tg.run("init")

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
	manifest := tg.readManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit := tg.getCommit("go.googlesource.com/sys")
	expectedLock := `{
    "memo": "e5aa3024d5de3a019bf6541029effdcd434538399eb079f432635c8524d31238",
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
	lock := tg.readLock()
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}
