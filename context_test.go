// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"path/filepath"
	"testing"

	"github.com/golang/dep/test"
	"github.com/sdboyer/gps"
)

func TestNewContextNoGOPATH(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Cd(h.Path("."))

	c, err := NewContext()
	if err == nil {
		t.Fatal("error should not have been nil")
	}

	if c != nil {
		t.Fatalf("expected context to be nil, got: %#v", c)
	}
}

func TestSplitAbsoluteProjectRoot(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))
	depCtx := &Ctx{GOPATH: h.Path(".")}

	importPaths := []string{
		"github.com/pkg/errors",
		"my/silly/thing",
	}

	for _, ip := range importPaths {
		fullpath := filepath.Join(depCtx.GOPATH, "src", ip)
		pr, err := depCtx.SplitAbsoluteProjectRoot(fullpath)
		if err != nil {
			t.Fatal(err)
		}
		if pr != ip {
			t.Fatalf("expected %s, got %s", ip, pr)
		}
	}

	// test where it should return error
	pr, err := depCtx.SplitAbsoluteProjectRoot("tra/la/la/la")
	if err == nil {
		t.Fatalf("should have gotten error but did not for tra/la/la/la: %s", pr)
	}
}

func TestAbsoluteProjectRoot(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))
	depCtx := &Ctx{GOPATH: h.Path(".")}

	importPaths := map[string]bool{
		"github.com/pkg/errors": true,
		"my/silly/thing":        false,
	}

	for i, create := range importPaths {
		if create {
			h.TempDir(filepath.Join("src", i))
		}
	}

	for i, ok := range importPaths {
		pr, err := depCtx.absoluteProjectRoot(i)
		if ok {
			h.Must(err)
			expected := h.Path(filepath.Join("src", i))
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
	h.TempFile("src/thing/thing.go", "hello world")
	_, err := depCtx.absoluteProjectRoot("thing/thing.go")
	if err == nil {
		t.Fatal("error should not be nil for a file found")
	}
}

func TestVersionInWorkspace(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.Setenv("GOPATH", h.Path("."))
	depCtx := &Ctx{GOPATH: h.Path(".")}

	importPaths := map[string]struct {
		rev      gps.Version
		checkout bool
	}{
		"github.com/pkg/errors": {
			rev:      gps.NewVersion("v0.8.0").Is("645ef00459ed84a119197bfb8d8205042c6df63d"), // semver
			checkout: true,
		},
		"github.com/Sirupsen/logrus": {
			rev:      gps.Revision("42b84f9ec624953ecbf81a94feccb3f5935c5edf"), // random sha
			checkout: true,
		},
		"github.com/rsc/go-get-default-branch": {
			rev: gps.NewBranch("another-branch").Is("8e6902fdd0361e8fa30226b350e62973e3625ed5"),
		},
	}

	// checkout the specified revisions
	for ip, info := range importPaths {
		h.RunGo("get", ip)
		repoDir := h.Path("src/" + ip)
		if info.checkout {
			h.RunGit(repoDir, "checkout", info.rev.String())
		}

		v, err := depCtx.VersionInWorkspace(gps.ProjectRoot(ip))
		h.Must(err)

		if v != info.rev {
			t.Fatalf("expected %q, got %q", v.String(), info.rev.String())
		}
	}
}

func TestLoadProjectWD(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempFile("src/prj/manifest.json", "{}")
	tg.Cd(tg.Path("src/prj"))
	depCtx := &Ctx{GOPATH: tg.Path(".")}
	tg.Setenv("GOPATH", depCtx.GOPATH)

	p1, err := depCtx.LoadProject("")
	if err != nil {
		t.Fatal("error should have been nil. Got:", err)
	}

	if p1 == nil {
		t.Fatal("project should not have been nil")
	}

	tg.TempFile("src/prj/lock.json", "{}")

	p2, err := depCtx.LoadProject("")
	if err != nil {
		t.Fatal("error should have been nil. Got:", err)
	}

	if p2 == nil {
		t.Fatal("project should not have been nil")
	}

}

func TestLoadProjectPath(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempFile("src/prj/manifest.json", "{}")
	depCtx := &Ctx{GOPATH: tg.Path(".")}
	tg.Setenv("GOPATH", depCtx.GOPATH)

	p1, err := depCtx.LoadProject(tg.Path("src/prj"))
	if err != nil {
		t.Fatal("error should have been nil. Got:", err)
	}

	if p1 == nil {
		t.Fatal("project should not have been nil")
	}

	tg.TempFile("src/prj/lock.json", "{}")

	p2, err := depCtx.LoadProject(tg.Path("src/prj"))
	if err != nil {
		t.Fatal("error should have been nil. Got:", err)
	}

	if p2 == nil {
		t.Fatal("project should not have been nil")
	}
}

func TestLoadProjectInvalidFiles(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempFile("src/prj/manifest.json", "--- # Invalid manifest")
	depCtx := &Ctx{GOPATH: tg.Path(".")}
	tg.Setenv("GOPATH", depCtx.GOPATH)

	p1, err := depCtx.LoadProject(tg.Path("src/prj"))
	if err == nil {
		t.Fatal("error should not have been nil. Got:", err)
	}

	if p1 != nil {
		t.Fatal("project should have been nil")
	}

	tg.TempFile("src/prj/manifest.json", "{}")
	tg.TempFile("src/prj/lock.json", "--- # Invalid lock")

	p2, err := depCtx.LoadProject(tg.Path("src/prj"))
	if err == nil {
		t.Fatal("error should not have been nil. Got:", err)
	}

	if p2 != nil {
		t.Fatal("project should have been nil")
	}

}
