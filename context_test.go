// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/dep/test"
	"github.com/sdboyer/gps"
)

func TestNewContextNoGOPATH(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.Cd(tg.Path("."))

	c, err := NewContext()
	if err == nil {
		t.Fatal("error should not have been nil")
	}

	if c != nil {
		t.Fatalf("expected context to be nil, got: %#v", c)
	}
}

func TestSplitAbsoluteProjectRoot(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.Setenv("GOPATH", tg.Path("."))
	depCtx := &Ctx{GOPATH: tg.Path(".")}

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
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.Setenv("GOPATH", tg.Path("."))
	depCtx := &Ctx{GOPATH: tg.Path(".")}

	importPaths := map[string]bool{
		"github.com/pkg/errors": true,
		"my/silly/thing":        false,
	}

	for i, create := range importPaths {
		if create {
			tg.TempDir(filepath.Join("src", i))
		}
	}

	for i, ok := range importPaths {
		pr, err := depCtx.absoluteProjectRoot(i)
		if ok {
			tg.Must(err)
			expected := tg.Path(filepath.Join("src", i))
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
	tg.TempFile("src/thing/thing.go", "hello world")
	_, err := depCtx.absoluteProjectRoot("thing/thing.go")
	if err == nil {
		t.Fatal("error should not be nil for a file found")
	}
}

func TestVersionInWorkspace(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.Setenv("GOPATH", tg.Path("."))
	depCtx := &Ctx{GOPATH: tg.Path(".")}

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
		tg.RunGo("get", ip)
		repoDir := tg.Path("src/" + ip)
		if info.checkout {
			tg.RunGit(repoDir, "checkout", info.rev.String())
		}

		v, err := depCtx.VersionInWorkspace(gps.ProjectRoot(ip))
		tg.Must(err)

		if v != info.rev {
			t.Fatalf("expected %q, got %q", v.String(), info.rev.String())
		}
	}
}

func TestLoadProject(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.TempDir("src/test1")
	tg.TempDir("src/test1/sub")
	tg.TempFile("src/test1/manifest.json", `{"dependencies":{}}`)
	tg.TempFile("src/test1/lock.json", `{"memo":"cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee","projects":[]}`)
	tg.TempDir("src/test2")
	tg.TempDir("src/test2/sub")
	tg.TempFile("src/test2/manifest.json", `{"dependencies":{}}`)
	tg.Setenv("GOPATH", tg.Path("."))

	var testcases = []struct {
		lock  bool
		start string
		path  string
	}{
		{true, filepath.Join("src", "test1"), ""},                       //empty path, direct
		{true, filepath.Join("src", "test1", "sub"), ""},                //empty path, ascending
		{true, ".", filepath.Join(tg.Path("."), "src", "test1")},        //absolute path, direct
		{true, ".", filepath.Join(tg.Path("."), "src", "test1", "sub")}, //absolute path, ascending
		{true, ".", filepath.Join("src", "test1")},                      //relative path from wd, direct
		{true, ".", filepath.Join("src", "test1", "sub")},               //relative path from wd, ascending
		{true, "src", "test1"},                                          //relative path from relative path, direct
		{true, "src", filepath.Join("test1", "sub")},                    //relative path from relative path, ascending
		{false, filepath.Join("src", "test2"), ""},                      //repeat without lockfile present
		{false, filepath.Join("src", "test2", "sub"), ""},
		{false, ".", filepath.Join(tg.Path("."), "src", "test2")},
		{false, ".", filepath.Join(tg.Path("."), "src", "test2", "sub")},
		{false, ".", filepath.Join("src", "test2")},
		{false, ".", filepath.Join("src", "test2", "sub")},
		{false, "src", "test2"},
		{false, "src", filepath.Join("test2", "sub")},
	}

	for _, testcase := range testcases {
		ctx := &Ctx{GOPATH: tg.Path(".")}

		start := testcase.start
		path := testcase.path
		tg.Cd(tg.Path(start))
		proj, err := ctx.LoadProject(path)
		tg.Must(err)
		if proj.Manifest == nil {
			t.Fatalf("Manifest file didn't load -> from: %q, path: %q", start, path)
		}
		if testcase.lock && proj.Lock == nil {
			t.Fatalf("Lock file didn't load -> from: %q, path: %q", start, path)
		} else if !testcase.lock && proj.Lock != nil {
			t.Fatalf("Non-existent Lock file loaded -> from: %q, path: %q", start, path)
		}
	}
}

func TestLoadProjectNotFoundErrors(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.TempDir("src/test1")
	tg.TempDir("src/test1/sub")
	tg.Setenv("GOPATH", tg.Path("."))

	var testcases = []struct {
		lock  bool
		start string
		path  string
	}{
		{true, filepath.Join("src", "test1"), ""},                       //empty path, direct
		{true, filepath.Join("src", "test1", "sub"), ""},                //empty path, ascending
		{true, ".", filepath.Join(tg.Path("."), "src", "test1")},        //absolute path, direct
		{true, ".", filepath.Join(tg.Path("."), "src", "test1", "sub")}, //absolute path, ascending
		{true, ".", filepath.Join("src", "test1")},                      //relative path from wd, direct
		{true, ".", filepath.Join("src", "test1", "sub")},               //relative path from wd, ascending
		{true, "src", "test1"},                                          //relative path from relative path, direct
		{true, "src", filepath.Join("test1", "sub")},                    //relative path from relative path, ascending
	}

	for _, testcase := range testcases {
		ctx := &Ctx{GOPATH: tg.Path(".")}

		start := testcase.start
		path := testcase.path
		tg.Cd(tg.Path(start))
		_, err := ctx.LoadProject(path)
		if err == nil {
			t.Fatalf("should have returned 'No Manifest Found' error -> from: %q, path: %q", start, path)
		}
	}
}

func TestLoadProjectManifestParseError(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.TempDir("src/test1")
	tg.TempFile("src/test1/manifest.json", ` "dependencies":{} `)
	tg.TempFile("src/test1/lock.json", `{"memo":"cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee","projects":[]}`)
	tg.Setenv("GOPATH", tg.Path("."))

	ctx := &Ctx{GOPATH: tg.Path(".")}
	path := filepath.Join("src", "test1")
	tg.Cd(tg.Path(path))

	_, err := ctx.LoadProject("")
	if err == nil {
		t.Fatalf("should have returned 'Manifest Syntax' error")
	}
}

func TestLoadProjectLockParseError(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.TempDir("src/test1")
	tg.TempFile("src/test1/manifest.json", `{"dependencies":{}}`)
	tg.TempFile("src/test1/lock.json", ` "memo":"cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee","projects":[] `)
	tg.Setenv("GOPATH", tg.Path("."))

	ctx := &Ctx{GOPATH: tg.Path(".")}
	path := filepath.Join("src", "test1")
	tg.Cd(tg.Path(path))

	_, err := ctx.LoadProject("")
	if err == nil {
		t.Fatalf("should have returned 'Lock Syntax' error")
	}
}

func TestLoadProjectNoSrcDir(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("test1")
	tg.TempFile("test1/manifest.json", `{"dependencies":{}}`)
	tg.TempFile("test1/lock.json", `{"memo":"cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee","projects":[]}`)
	tg.Setenv("GOPATH", tg.Path("."))

	ctx := &Ctx{GOPATH: tg.Path(".")}
	path := filepath.Join("test1")
	tg.Cd(tg.Path(path))

	f, _ := os.OpenFile(filepath.Join(ctx.GOPATH, "src", "test1", "lock.json"), os.O_WRONLY, os.ModePerm)
	defer f.Close()

	_, err := ctx.LoadProject("")
	if err == nil {
		t.Fatalf("should have returned 'Split Absolute Root' error (no 'src' dir present)")
	}
}
