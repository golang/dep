// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/golang/dep/test"
)

func TestRemove(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.Setenv("GOPATH", tg.Path("."))

	importPaths := map[string]string{
		"github.com/pkg/errors":      "v0.8.0",                                   // semver
		"github.com/Sirupsen/logrus": "42b84f9ec624953ecbf81a94feccb3f5935c5edf", // random sha
	}

	// checkout the specified revisions
	for ip, rev := range importPaths {
		tg.RunGo("get", ip)
		repoDir := tg.Path("src/" + ip)
		tg.RunGit(repoDir, "checkout", rev)
	}

	// Build a fake consumer of these packages.
	const root = "github.com/golang/notexist"
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
	logrus.Info("whatev")
}`

	tg.TempFile("src/"+root+"/thing.go", m)
	origm := `{
    "dependencies": {
        "github.com/not/used": {
            "version": "2.0.0"
        },
        "github.com/Sirupsen/logrus": {
            "revision": "42b84f9ec624953ecbf81a94feccb3f5935c5edf"
        },
        "github.com/pkg/errors": {
            "version": ">=0.8.0, <1.0.0"
        }
    }
}
`
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

	tg.TempFile("src/"+root+"/manifest.json", origm)

	tg.Cd(tg.Path("src/" + root))
	tg.Run("remove", "-unused")

	manifest := tg.ReadManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	tg.TempFile("src/"+root+"/manifest.json", origm)
	tg.Run("remove", "github.com/not/used")

	manifest = tg.ReadManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	if err := tg.DoRun([]string{"remove", "-unused", "github.com/not/used"}); err == nil {
		t.Fatal("rm with both -unused and arg should have failed")
	}

	if err := tg.DoRun([]string{"remove", "github.com/not/present"}); err == nil {
		t.Fatal("rm with arg not in manifest should have failed")
	}

	if err := tg.DoRun([]string{"remove", "github.com/not/used", "github.com/not/present"}); err == nil {
		t.Fatal("rm with one arg not in manifest should have failed")
	}

	if err := tg.DoRun([]string{"remove", "github.com/pkg/errors"}); err == nil {
		t.Fatal("rm of arg in manifest and imports should have failed without -force")
	}

	tg.TempFile("src/"+root+"/manifest.json", origm)
	tg.Run("remove", "-force", "github.com/pkg/errors", "github.com/not/used")

	manifest = tg.ReadManifest()
	if manifest != `{
    "dependencies": {
        "github.com/Sirupsen/logrus": {
            "revision": "42b84f9ec624953ecbf81a94feccb3f5935c5edf"
        }
    }
}
` {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit := tg.GetCommit("go.googlesource.com/sys")
	expectedLock := `{
    "memo": "7769242a737ed497aa39831eecfdc4a1bf59517df898907accc6bdc0f789a69b",
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
	lock := tg.ReadLock()
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}
