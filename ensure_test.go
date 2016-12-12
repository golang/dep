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

	sysCommit, err := getRepoLatestCommit("golang/sys")
	tg.must(err)
	expectedLock := `{
    "memo": "7a56f62368179fde8c13c3880e25f9e0aa55bceafc61abef6640ecb0c4d63d88",
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

func TestDeduceConstraint(t *testing.T) {
	sv, err := gps.NewSemverConstraint("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	constraints := map[string]gps.Constraint{
		"v1.2.3": sv,
		"5b3352dc16517996fb951394bcbbe913a2a616e3":      gps.Revision("5b3352dc16517996fb951394bcbbe913a2a616e3"),
		"g4@golang.org-20161116211307-wiuilyamo9ian0m7": gps.NewVersion("g4@golang.org-20161116211307-wiuilyamo9ian0m7"),
	}

	for str, expected := range constraints {
		c := deduceConstraint(str)
		if c != expected {
			t.Fatalf("expected: %#v, got %#v for %s", expected, c, str)
		}
	}
}

func TestCopyFolder(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcf, err := os.Create(filepath.Join(dir, "myfile"))
	if err != nil {
		t.Fatal(err)
	}

	contents := "hello world"
	if _, err := srcf.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	destdir := dir + "more-temp"
	if err := copyFolder(dir, destdir); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(destdir)

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
	srcf, err := ioutil.TempFile("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(srcf.Name())

	contents := "hello world"
	if _, err := srcf.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	destf := srcf.Name() + "more-temp"
	if err := copyFile(srcf.Name(), destf); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(destf)

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
