package main

import "testing"

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
            "revision": "478fcf54317e52ab69f40bb4c7a1520288d7f7ea",
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
