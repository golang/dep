// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sdboyer/gps"
)

func TestEnsureOverrides(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))

	m := `package main

import (
	"github.com/Sirupsen/logrus"
	sthing "github.com/sdboyer/dep-test"
)

type Baz sthing.Foo

func main() {
	logrus.Info("hello world")
}`

	tg.tempFile("src/thing/thing.go", m)
	tg.cd(tg.path("src/thing"))

	tg.run("init")
	tg.run("ensure", "-override", "github.com/Sirupsen/logrus@0.11.0")

	expectedManifest := `{
    "overrides": {
        "github.com/Sirupsen/logrus": {
            "version": "0.11.0"
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
	lock := tg.readLock()
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}

func TestEnsureEmptyRepoNoArgs(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))

	m := `package main

import (
	"github.com/Sirupsen/logrus"
)

func main() {
	logrus.Info("hello world")
}`

	tg.tempFile("src/thing/thing.go", m)
	tg.cd(tg.path("src/thing"))

	tg.run("init")
	tg.run("ensure")

	// make sure vendor exists
	tg.mustExist(tg.path("src/thing/vendor/github.com/Sirupsen/logrus"))

	expectedManifest := `{}
`

	manifest := tg.readManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit := tg.getCommit("go.googlesource.com/sys")
	logrusCommit := tg.getCommit("github.com/Sirupsen/logrus")
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

	lock := tg.readLock()
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

func TestCopyDir(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcdir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcdir, 0755); err != nil {
		t.Fatal(err)
	}

	srcf, err := os.Create(filepath.Join(srcdir, "myfile"))
	if err != nil {
		t.Fatal(err)
	}

	contents := "hello world"
	if _, err := srcf.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	destdir := filepath.Join(dir, "dest")
	if err := copyDir(srcdir, destdir); err != nil {
		t.Fatal(err)
	}

	dirOK, err := isDir(destdir)
	if err != nil {
		t.Fatal(err)
	}
	if !dirOK {
		t.Fatalf("expected %s to be a directory", destdir)
	}

	destf := filepath.Join(destdir, "myfile")
	destcontents, err := ioutil.ReadFile(destf)
	if err != nil {
		t.Fatal(err)
	}

	if contents != string(destcontents) {
		t.Fatalf("expected: %s, got: %s", contents, string(destcontents))
	}

	srcinfo, err := os.Stat(srcf.Name())
	if err != nil {
		t.Fatal(err)
	}

	destinfo, err := os.Stat(destf)
	if err != nil {
		t.Fatal(err)
	}

	if srcinfo.Mode() != destinfo.Mode() {
		t.Fatalf("expected %s: %#v\n to be the same mode as %s: %#v", srcf.Name(), srcinfo.Mode(), destf, destinfo.Mode())
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

	contents := "hello world"
	if _, err := srcf.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	destf := filepath.Join(dir, "destf")
	if err := copyFile(srcf.Name(), destf); err != nil {
		t.Fatal(err)
	}

	destcontents, err := ioutil.ReadFile(destf)
	if err != nil {
		t.Fatal(err)
	}

	if contents != string(destcontents) {
		t.Fatalf("expected: %s, got: %s", contents, string(destcontents))
	}

	srcinfo, err := os.Stat(srcf.Name())
	if err != nil {
		t.Fatal(err)
	}

	destinfo, err := os.Stat(destf)
	if err != nil {
		t.Fatal(err)
	}

	if srcinfo.Mode() != destinfo.Mode() {
		t.Fatalf("expected %s: %#v\n to be the same mode as %s: %#v", srcf.Name(), srcinfo.Mode(), destf, destinfo.Mode())
	}
}
