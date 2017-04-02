// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"

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

	for _, want := range importPaths {
		fullpath := filepath.Join(depCtx.GOPATH, "src", want)
		got, err := depCtx.SplitAbsoluteProjectRoot(fullpath)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("expected %s, got %s", want, got)
		}
	}

	// test where it should return error
	got, err := depCtx.SplitAbsoluteProjectRoot("tra/la/la/la")
	if err == nil {
		t.Fatalf("should have gotten error but did not for tra/la/la/la: %s", got)
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
		got, err := depCtx.absoluteProjectRoot(i)
		if ok {
			h.Must(err)
			want := h.Path(filepath.Join("src", i))
			if got != want {
				t.Fatalf("expected %s, got %q", want, got)
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

		got, err := depCtx.VersionInWorkspace(gps.ProjectRoot(ip))
		h.Must(err)

		if got != info.rev {
			t.Fatalf("expected %q, got %q", got.String(), info.rev.String())
		}
	}
}

func TestLoadProject(t *testing.T) {
	tg := test.NewHelper(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.TempDir("src/test1")
	tg.TempDir("src/test1/sub")
	tg.TempFile(filepath.Join("src/test1", ManifestName), "")
	tg.TempFile(filepath.Join("src/test1", LockName), `memo = "cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee"`)
	tg.TempDir("src/test2")
	tg.TempDir("src/test2/sub")
	tg.TempFile(filepath.Join("src/test2", ManifestName), "")
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
	tg := test.NewHelper(t)
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
	tg := test.NewHelper(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.TempDir("src/test1")
	tg.TempFile(filepath.Join("src/test1", ManifestName), `[[dependencies]]`)
	tg.TempFile(filepath.Join("src/test1", LockName), `memo = "cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee"\n\n[[projects]]`)
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
	tg := test.NewHelper(t)
	defer tg.Cleanup()

	tg.TempDir("src")
	tg.TempDir("src/test1")
	tg.TempFile(filepath.Join("src/test1", ManifestName), `[[dependencies]]`)
	tg.TempFile(filepath.Join("src/test1", LockName), `memo = "cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee"\n\n[[projects]]`)
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
	tg := test.NewHelper(t)
	defer tg.Cleanup()

	tg.TempDir("test1")
	tg.TempFile(filepath.Join("test1", ManifestName), `[[dependencies]]`)
	tg.TempFile(filepath.Join("test1", LockName), `memo = "cdafe8641b28cd16fe025df278b0a49b9416859345d8b6ba0ace0272b74925ee"\n\n[[projects]]`)
	tg.Setenv("GOPATH", tg.Path("."))

	ctx := &Ctx{GOPATH: tg.Path(".")}
	path := filepath.Join("test1")
	tg.Cd(tg.Path(path))

	f, _ := os.OpenFile(filepath.Join(ctx.GOPATH, "src", "test1", LockName), os.O_WRONLY, os.ModePerm)
	defer f.Close()

	_, err := ctx.LoadProject("")
	if err == nil {
		t.Fatalf("should have returned 'Split Absolute Root' error (no 'src' dir present)")
	}
}

// TestCaseInsentitive is test for Windows. This should work even though set
// difference letter cases in GOPATH.
func TestCaseInsentitiveGOPATH(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skip this test on non-Windows")
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.TempDir("src/test1")
	h.TempFile(filepath.Join("src/test1", ManifestName), `[[dependencies]]`)

	// Shuffle letter case
	rs := []rune(strings.ToLower(h.Path(".")))
	for i, r := range rs {
		if unicode.IsLower(r) {
			rs[i] = unicode.ToUpper(r)
		} else {
			rs[i] = unicode.ToLower(r)
		}
	}
	gopath := string(rs)
	h.Setenv("GOPATH", gopath)
	depCtx := &Ctx{GOPATH: gopath}

	depCtx.LoadProject("")

	ip := "github.com/pkg/errors"
	fullpath := filepath.Join(depCtx.GOPATH, "src", ip)
	pr, err := depCtx.SplitAbsoluteProjectRoot(fullpath)
	if err != nil {
		t.Fatal(err)
	}
	if pr != ip {
		t.Fatalf("expected %s, got %s", ip, pr)
	}
}
