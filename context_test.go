package main

import (
	"path/filepath"
	"testing"

	"github.com/sdboyer/gps"
)

func TestNewContextNoGOPATH(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.cd(tg.path("."))

	c, err := newContext()
	if err == nil {
		t.Fatal("error should not have been nil")
	}

	if c != nil {
		t.Fatalf("expected context to be nil, got: %#v", c)
	}
}

func TestSplitAbsoluteProjectRoot(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))
	hoardCtx := &ctx{GOPATH: tg.path(".")}

	importPaths := []string{
		"github.com/pkg/errors",
		"my/silly/thing",
	}

	for _, ip := range importPaths {
		fullpath := filepath.Join(hoardCtx.GOPATH, "src", ip)
		pr, err := hoardCtx.splitAbsoluteProjectRoot(fullpath)
		if err != nil {
			t.Fatal(err)
		}
		if pr != ip {
			t.Fatalf("expected %s, got %s", ip, pr)
		}
	}

	// test where it should return error
	pr, err := hoardCtx.splitAbsoluteProjectRoot("tra/la/la/la")
	if err == nil {
		t.Fatalf("should have gotten error but did not for tra/la/la/la: %s", pr)
	}
}

func TestAbsoluteProjectRoot(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))
	hoardCtx := &ctx{GOPATH: tg.path(".")}

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
		pr, err := hoardCtx.absoluteProjectRoot(i)
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
	_, err := hoardCtx.absoluteProjectRoot("thing/thing.go")
	if err == nil {
		t.Fatal("error should not be nil for a file found")
	}
}

func TestVersionInWorkspace(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))
	hoardCtx := &ctx{GOPATH: tg.path(".")}

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
		tg.runGo("get", ip)
		repoDir := tg.path("src/" + ip)
		if info.checkout {
			tg.runGit(repoDir, "checkout", info.rev.String())
		}

		v, err := hoardCtx.versionInWorkspace(gps.ProjectRoot(ip))
		tg.must(err)

		if v != info.rev {
			t.Fatalf("expected %q, got %q", v.String(), info.rev.String())
		}
	}
}
