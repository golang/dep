// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestRemove(t *testing.T) {
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

	tg.tempFile("src/"+root+"/thing.go", m)
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

	tg.tempFile("src/"+root+"/manifest.json", origm)

	tg.cd(tg.path("src/" + root))
	tg.run("remove", "-unused")

	manifest := tg.readManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	tg.tempFile("src/"+root+"/manifest.json", origm)
	tg.run("remove", "github.com/not/used")

	manifest = tg.readManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	if err := tg.doRun([]string{"remove", "-unused", "github.com/not/used"}); err == nil {
		t.Fatal("rm with both -unused and arg should have failed")
	}

	if err := tg.doRun([]string{"remove", "github.com/not/present"}); err == nil {
		t.Fatal("rm with arg not in manifest should have failed")
	}

	if err := tg.doRun([]string{"remove", "github.com/not/used", "github.com/not/present"}); err == nil {
		t.Fatal("rm with one arg not in manifest should have failed")
	}

	if err := tg.doRun([]string{"remove", "github.com/pkg/errors"}); err == nil {
		t.Fatal("rm of arg in manifest and imports should have failed without -force")
	}

	tg.tempFile("src/"+root+"/manifest.json", origm)
	tg.run("remove", "-force", "github.com/pkg/errors", "github.com/not/used")

	manifest = tg.readManifest()
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

	sysCommit := tg.getCommit("go.googlesource.com/sys")
	expectedLock := `{
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
	lock := wipeMemo(tg.readLock())
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}
