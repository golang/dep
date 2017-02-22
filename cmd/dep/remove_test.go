// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"

	"github.com/golang/dep/test"
)

func TestRemove(t *testing.T) {
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
	const root = "src/github.com/golang/notexist"
	h.TempCopy(root+"/thing.go", "remove/main.input.go")
	h.TempCopy(root+"/manifest.json", "remove/manifest.input.json")

	h.Cd(h.Path(root))
	h.Run("remove", "-unused")

	goldenManifest := "remove/manifest0.golden.json"
	wantManifest := h.GetTestFileString(goldenManifest)
	gotManifest := h.ReadManifest()
	if wantManifest != gotManifest {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenManifest, gotManifest); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("expected %s, got %s", wantManifest, gotManifest)
		}
	}

	h.TempCopy(root+"/manifest.json", "remove/manifest.input.json")
	h.Run("remove", "github.com/not/used")

	gotManifest = h.ReadManifest()
	if wantManifest != gotManifest {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenManifest, gotManifest); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("expected %s, got %s", wantManifest, gotManifest)
		}
	}

	if err := h.DoRun([]string{"remove", "-unused", "github.com/not/used"}); err == nil {
		t.Fatal("rm with both -unused and arg should have failed")
	}

	if err := h.DoRun([]string{"remove", "github.com/not/present"}); err == nil {
		t.Fatal("rm with arg not in manifest should have failed")
	}

	if err := h.DoRun([]string{"remove", "github.com/not/used", "github.com/not/present"}); err == nil {
		t.Fatal("rm with one arg not in manifest should have failed")
	}

	if err := h.DoRun([]string{"remove", "github.com/pkg/errors"}); err == nil {
		t.Fatal("rm of arg in manifest and imports should have failed without -force")
	}

	h.TempCopy(root+"/manifest.json", "remove/manifest.input.json")
	h.Run("remove", "-force", "github.com/pkg/errors", "github.com/not/used")

	goldenManifest = "remove/manifest1.golden.json"
	wantManifest = h.GetTestFileString(goldenManifest)
	gotManifest = h.ReadManifest()
	if wantManifest != gotManifest {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenManifest, gotManifest); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("expected %s, got %s", wantManifest, gotManifest)
		}
	}

	sysCommit := h.GetCommit("go.googlesource.com/sys")
	goldenLock := "remove/lock1.golden.json"
	wantLock := strings.Replace(h.GetTestFileString(goldenLock), "{{sysCommit}}", sysCommit, 1)
	gotLock := h.ReadLock()
	if wantLock != gotLock {
		if *test.UpdateGolden {
			if err := h.WriteTestFile(goldenLock, strings.Replace(gotLock, sysCommit, "{{sysCommit}}", 1)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("expected %s, got %s", wantLock, gotLock)
		}
	}
}
