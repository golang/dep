package main

import (
	"path/filepath"
	"testing"

	"github.com/sdboyer/gps"
)

func TestAbsoluteProjectRoot(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))
	depCtx := &ctx{GOPATH: tg.path(".")}

	importPaths := map[string]bool{
		"github.com/pkg/errors": true,
		"my/silly/thing":        false,
	}

	for i, create := range importPaths {
		if create {
			tg.tempDir(filepath.Join("src", i))
		}
	}

	for i, ok := range importPaths {
		pr, err := depCtx.absoluteProjectRoot(i)
		if ok {
			tg.must(err)
			expected := tg.path(filepath.Join("src", i))
			if pr != expected {
				t.Fatalf("expected %s, got %q", expected, pr)
			}
			continue
		}

		if err == nil {
			t.Fatalf("expected %s to fail", i)
		}
	}

	// test that a file fails
	tg.tempFile("src/thing/thing.go", "hello world")
	_, err := depCtx.absoluteProjectRoot("thing/thing.go")
	if err == nil {
		t.Fatal("error should not be nil for a file found")
	}
}

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

func TestVersionInWorkspace(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))
	depCtx := &ctx{GOPATH: tg.path(".")}

	importPaths := map[string]gps.Version{
		"github.com/pkg/errors":      gps.NewVersion("v0.8.0").Is("645ef00459ed84a119197bfb8d8205042c6df63d"), // semver
		"github.com/Sirupsen/logrus": gps.Revision("42b84f9ec624953ecbf81a94feccb3f5935c5edf"),                // random sha
	}

	// checkout the specified revisions
	for ip, rev := range importPaths {
		tg.runGo("get", ip)
		repoDir := tg.path("src/" + ip)
		tg.runGit(repoDir, "checkout", rev.String())

		v, err := depCtx.versionInWorkspace(gps.ProjectRoot(ip))
		tg.must(err)

		if v != rev {
			t.Fatalf("expected %s, got %s", v.String(), rev.String())
		}
	}
}

func TestInit(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

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

	m := `package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

func main() {
	err := nil
	if err != nil {
		errors.Wrap(err, "thing")
	}
	logrus.Info("hello world")
}`

	tg.tempFile("src/thing/thing.go", m)
	tg.cd(tg.path("src/thing"))

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

	expectedLock := `{
    "memo": "0d682aa197bf6e94b9af8298acea5b7957b9a5674570cc16b3e8436846928a57",
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
