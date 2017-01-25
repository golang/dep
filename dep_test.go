// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	exeSuffix string // ".exe" on Windows
	mu        sync.Mutex
)

func init() {
	switch runtime.GOOS {
	case "windows":
		exeSuffix = ".exe"
	}
}

// The TestMain function creates a dep command for testing purposes and
// deletes it after the tests have been run.
// Most of this is taken from https://github.com/golang/go/blob/master/src/cmd/go/go_test.go and reused here.
func TestMain(m *testing.M) {
	args := []string{"build", "-o", "testdep" + exeSuffix}
	out, err := exec.Command("go", args...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "building testdep failed: %v\n%s", err, out)
		os.Exit(2)
	}

	// Don't let these environment variables confuse the test.
	os.Unsetenv("GOPATH")
	os.Unsetenv("GIT_ALLOW_PROTOCOL")
	if home, ccacheDir := os.Getenv("HOME"), os.Getenv("CCACHE_DIR"); home != "" && ccacheDir == "" {
		// On some systems the default C compiler is ccache.
		// Setting HOME to a non-existent directory will break
		// those systems.  Set CCACHE_DIR to cope.  Issue 17668.
		os.Setenv("CCACHE_DIR", filepath.Join(home, ".ccache"))
	}
	os.Setenv("HOME", "/test-dep-home-does-not-exist")

	r := m.Run()

	os.Remove("testdep" + exeSuffix)

	os.Exit(r)
}

// Manage a single run of the testgo binary.
type testgoData struct {
	t              *testing.T
	temps          []string
	wd             string
	env            []string
	tempdir        string
	ran            bool
	inParallel     bool
	stdout, stderr bytes.Buffer
}

// testgo sets up for a test that runs testgo.
func testgo(t *testing.T) *testgoData {
	return &testgoData{t: t}
}

// must gives a fatal error if err is not nil.
func (tg *testgoData) must(err error) {
	if err != nil {
		tg.t.Fatal(err)
	}
}

// check gives a test non-fatal error if err is not nil.
func (tg *testgoData) check(err error) {
	if err != nil {
		tg.t.Error(err)
	}
}

// parallel runs the test in parallel by calling t.Parallel.
func (tg *testgoData) parallel() {
	if tg.ran {
		tg.t.Fatal("internal testsuite error: call to parallel after run")
	}
	if tg.wd != "" {
		tg.t.Fatal("internal testsuite error: call to parallel after cd")
	}
	for _, e := range tg.env {
		if strings.HasPrefix(e, "GOROOT=") || strings.HasPrefix(e, "GOPATH=") || strings.HasPrefix(e, "GOBIN=") {
			val := e[strings.Index(e, "=")+1:]
			if strings.HasPrefix(val, "testdata") || strings.HasPrefix(val, "./testdata") {
				tg.t.Fatalf("internal testsuite error: call to parallel with testdata in environment (%s)", e)
			}
		}
	}
	tg.inParallel = true
	tg.t.Parallel()
}

// pwd returns the current directory.
func (tg *testgoData) pwd() string {
	wd, err := os.Getwd()
	if err != nil {
		tg.t.Fatalf("could not get working directory: %v", err)
	}
	return wd
}

// cd changes the current directory to the named directory. Note that
// using this means that the test must not be run in parallel with any
// other tests.
func (tg *testgoData) cd(dir string) {
	if tg.inParallel {
		tg.t.Fatal("internal testsuite error: changing directory when running in parallel")
	}
	if tg.wd == "" {
		tg.wd = tg.pwd()
	}
	abs, err := filepath.Abs(dir)
	tg.must(os.Chdir(dir))
	if err == nil {
		tg.setenv("PWD", abs)
	}
}

// setenv sets an environment variable to use when running the test go
// command.
func (tg *testgoData) setenv(name, val string) {
	if tg.inParallel && (name == "GOROOT" || name == "GOPATH" || name == "GOBIN") && (strings.HasPrefix(val, "testdata") || strings.HasPrefix(val, "./testdata")) {
		tg.t.Fatalf("internal testsuite error: call to setenv with testdata (%s=%s) after parallel", name, val)
	}
	tg.unsetenv(name)
	tg.env = append(tg.env, name+"="+val)
}

// unsetenv removes an environment variable.
func (tg *testgoData) unsetenv(name string) {
	if tg.env == nil {
		tg.env = append([]string(nil), os.Environ()...)
	}
	for i, v := range tg.env {
		if strings.HasPrefix(v, name+"=") {
			tg.env = append(tg.env[:i], tg.env[i+1:]...)
			break
		}
	}
}

// doRun runs the test go command, recording stdout and stderr and
// returning exit status.
func (tg *testgoData) doRun(args []string) error {
	if tg.inParallel {
		for _, arg := range args {
			if strings.HasPrefix(arg, "testdata") || strings.HasPrefix(arg, "./testdata") {
				tg.t.Fatal("internal testsuite error: parallel run using testdata")
			}
		}
	}
	tg.t.Logf("running testdep %v", args)
	var prog string
	if tg.wd == "" {
		prog = "./testdep" + exeSuffix
	} else {
		prog = filepath.Join(tg.wd, "testdep"+exeSuffix)
	}
	args = append(args[:1], append([]string{"-v"}, args[1:]...)...)
	cmd := exec.Command(prog, args...)
	tg.stdout.Reset()
	tg.stderr.Reset()
	cmd.Stdout = &tg.stdout
	cmd.Stderr = &tg.stderr
	cmd.Env = tg.env
	status := cmd.Run()
	if tg.stdout.Len() > 0 {
		tg.t.Log("standard output:")
		tg.t.Log(tg.stdout.String())
	}
	if tg.stderr.Len() > 0 {
		tg.t.Log("standard error:")
		tg.t.Log(tg.stderr.String())
	}
	tg.ran = true
	return status
}

// run runs the test go command, and expects it to succeed.
func (tg *testgoData) run(args ...string) {
	if runtime.GOOS == "windows" {
		mu.Lock()
		defer mu.Unlock()
	}
	if status := tg.doRun(args); status != nil {
		tg.t.Logf("go %v failed unexpectedly: %v", args, status)
		tg.t.FailNow()
	}
}

// runFail runs the test go command, and expects it to fail.
func (tg *testgoData) runFail(args ...string) {
	if status := tg.doRun(args); status == nil {
		tg.t.Fatal("testgo succeeded unexpectedly")
	} else {
		tg.t.Log("testgo failed as expected:", status)
	}
}

// runGo runs a go command, and expects it to succeed.
func (tg *testgoData) runGo(args ...string) {
	cmd := exec.Command("go", args...)
	tg.stdout.Reset()
	tg.stderr.Reset()
	cmd.Stdout = &tg.stdout
	cmd.Stderr = &tg.stderr
	cmd.Dir = tg.wd
	cmd.Env = tg.env
	status := cmd.Run()
	if tg.stdout.Len() > 0 {
		tg.t.Log("go standard output:")
		tg.t.Log(tg.stdout.String())
	}
	if tg.stderr.Len() > 0 {
		tg.t.Log("go standard error:")
		tg.t.Log(tg.stderr.String())
	}
	if status != nil {
		tg.t.Logf("go %v failed unexpectedly: %v", args, status)
		tg.t.FailNow()
	}
}

func needsExternalNetwork(t *testing.T) {
	if testing.Short() {
		t.Skipf("skipping test: no external network in -short mode")
	}
}

func needsGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skipping because git binary not found")
	}
}

// runGit runs a git command, and expects it to succeed.
func (tg *testgoData) runGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	tg.stdout.Reset()
	tg.stderr.Reset()
	cmd.Stdout = &tg.stdout
	cmd.Stderr = &tg.stderr
	cmd.Dir = dir
	cmd.Env = tg.env
	status := cmd.Run()
	if tg.stdout.Len() > 0 {
		tg.t.Logf("git %v standard output:", args)
		tg.t.Log(tg.stdout.String())
	}
	if tg.stderr.Len() > 0 {
		tg.t.Logf("git %v standard error:", args)
		tg.t.Log(tg.stderr.String())
	}
	if status != nil {
		tg.t.Logf("git %v failed unexpectedly: %v", args, status)
		tg.t.FailNow()
	}
}

// getStdout returns standard output of the testgo run as a string.
func (tg *testgoData) getStdout() string {
	if !tg.ran {
		tg.t.Fatal("internal testsuite error: stdout called before run")
	}
	return tg.stdout.String()
}

// getStderr returns standard error of the testgo run as a string.
func (tg *testgoData) getStderr() string {
	if !tg.ran {
		tg.t.Fatal("internal testsuite error: stdout called before run")
	}
	return tg.stderr.String()
}

// doGrepMatch looks for a regular expression in a buffer, and returns
// whether it is found. The regular expression is matched against
// each line separately, as with the grep command.
func (tg *testgoData) doGrepMatch(match string, b *bytes.Buffer) bool {
	if !tg.ran {
		tg.t.Fatal("internal testsuite error: grep called before run")
	}
	re := regexp.MustCompile(match)
	for _, ln := range bytes.Split(b.Bytes(), []byte{'\n'}) {
		if re.Match(ln) {
			return true
		}
	}
	return false
}

// doGrep looks for a regular expression in a buffer and fails if it
// is not found. The name argument is the name of the output we are
// searching, "output" or "error".  The msg argument is logged on
// failure.
func (tg *testgoData) doGrep(match string, b *bytes.Buffer, name, msg string) {
	if !tg.doGrepMatch(match, b) {
		tg.t.Log(msg)
		tg.t.Logf("pattern %v not found in standard %s", match, name)
		tg.t.FailNow()
	}
}

// grepStdout looks for a regular expression in the test run's
// standard output and fails, logging msg, if it is not found.
func (tg *testgoData) grepStdout(match, msg string) {
	tg.doGrep(match, &tg.stdout, "output", msg)
}

// grepStderr looks for a regular expression in the test run's
// standard error and fails, logging msg, if it is not found.
func (tg *testgoData) grepStderr(match, msg string) {
	tg.doGrep(match, &tg.stderr, "error", msg)
}

// grepBoth looks for a regular expression in the test run's standard
// output or stand error and fails, logging msg, if it is not found.
func (tg *testgoData) grepBoth(match, msg string) {
	if !tg.doGrepMatch(match, &tg.stdout) && !tg.doGrepMatch(match, &tg.stderr) {
		tg.t.Log(msg)
		tg.t.Logf("pattern %v not found in standard output or standard error", match)
		tg.t.FailNow()
	}
}

// doGrepNot looks for a regular expression in a buffer and fails if
// it is found. The name and msg arguments are as for doGrep.
func (tg *testgoData) doGrepNot(match string, b *bytes.Buffer, name, msg string) {
	if tg.doGrepMatch(match, b) {
		tg.t.Log(msg)
		tg.t.Logf("pattern %v found unexpectedly in standard %s", match, name)
		tg.t.FailNow()
	}
}

// grepStdoutNot looks for a regular expression in the test run's
// standard output and fails, logging msg, if it is found.
func (tg *testgoData) grepStdoutNot(match, msg string) {
	tg.doGrepNot(match, &tg.stdout, "output", msg)
}

// grepStderrNot looks for a regular expression in the test run's
// standard error and fails, logging msg, if it is found.
func (tg *testgoData) grepStderrNot(match, msg string) {
	tg.doGrepNot(match, &tg.stderr, "error", msg)
}

// grepBothNot looks for a regular expression in the test run's
// standard output or stand error and fails, logging msg, if it is
// found.
func (tg *testgoData) grepBothNot(match, msg string) {
	if tg.doGrepMatch(match, &tg.stdout) || tg.doGrepMatch(match, &tg.stderr) {
		tg.t.Log(msg)
		tg.t.Fatalf("pattern %v found unexpectedly in standard output or standard error", match)
	}
}

// doGrepCount counts the number of times a regexp is seen in a buffer.
func (tg *testgoData) doGrepCount(match string, b *bytes.Buffer) int {
	if !tg.ran {
		tg.t.Fatal("internal testsuite error: doGrepCount called before run")
	}
	re := regexp.MustCompile(match)
	c := 0
	for _, ln := range bytes.Split(b.Bytes(), []byte{'\n'}) {
		if re.Match(ln) {
			c++
		}
	}
	return c
}

// grepCountBoth returns the number of times a regexp is seen in both
// standard output and standard error.
func (tg *testgoData) grepCountBoth(match string) int {
	return tg.doGrepCount(match, &tg.stdout) + tg.doGrepCount(match, &tg.stderr)
}

// creatingTemp records that the test plans to create a temporary file
// or directory. If the file or directory exists already, it will be
// removed. When the test completes, the file or directory will be
// removed if it exists.
func (tg *testgoData) creatingTemp(path string) {
	if filepath.IsAbs(path) && !strings.HasPrefix(path, tg.tempdir) {
		tg.t.Fatalf("internal testsuite error: creatingTemp(%q) with absolute path not in temporary directory", path)
	}
	// If we have changed the working directory, make sure we have
	// an absolute path, because we are going to change directory
	// back before we remove the temporary.
	if tg.wd != "" && !filepath.IsAbs(path) {
		path = filepath.Join(tg.pwd(), path)
	}
	tg.must(os.RemoveAll(path))
	tg.temps = append(tg.temps, path)
}

// makeTempdir makes a temporary directory for a run of testgo. If
// the temporary directory was already created, this does nothing.
func (tg *testgoData) makeTempdir() {
	if tg.tempdir == "" {
		var err error
		tg.tempdir, err = ioutil.TempDir("", "gotest")
		tg.must(err)
	}
}

// tempFile adds a temporary file for a run of testgo.
func (tg *testgoData) tempFile(path, contents string) {
	tg.makeTempdir()
	tg.must(os.MkdirAll(filepath.Join(tg.tempdir, filepath.Dir(path)), 0755))
	bytes := []byte(contents)
	if strings.HasSuffix(path, ".go") {
		formatted, err := format.Source(bytes)
		if err == nil {
			bytes = formatted
		}
	}
	tg.must(ioutil.WriteFile(filepath.Join(tg.tempdir, path), bytes, 0644))
}

// tempDir adds a temporary directory for a run of testgo.
func (tg *testgoData) tempDir(path string) {
	tg.makeTempdir()
	if err := os.MkdirAll(filepath.Join(tg.tempdir, path), 0755); err != nil && !os.IsExist(err) {
		tg.t.Fatal(err)
	}
}

// path returns the absolute pathname to file with the temporary
// directory.
func (tg *testgoData) path(name string) string {
	if tg.tempdir == "" {
		tg.t.Fatalf("internal testsuite error: path(%q) with no tempdir", name)
	}
	if name == "." {
		return tg.tempdir
	}
	return filepath.Join(tg.tempdir, name)
}

// mustExist fails if path does not exist.
func (tg *testgoData) mustExist(path string) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			tg.t.Fatalf("%s does not exist but should", path)
		}
		tg.t.Fatalf("%s stat failed: %v", path, err)
	}
}

// mustNotExist fails if path exists.
func (tg *testgoData) mustNotExist(path string) {
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		tg.t.Fatalf("%s exists but should not (%v)", path, err)
	}
}

// cleanup cleans up a test that runs testgo.
func (tg *testgoData) cleanup() {
	if tg.wd != "" {
		if err := os.Chdir(tg.wd); err != nil {
			// We are unlikely to be able to continue.
			fmt.Fprintln(os.Stderr, "could not restore working directory, crashing:", err)
			os.Exit(2)
		}
	}
	// NOTE(mattn): It seems that sometimes git.exe is not dead
	// when cleanup() is called. But we do not know any way to wait for it.
	if runtime.GOOS == "windows" {
		mu.Lock()
		exec.Command(`taskkill`, `/F`, `/IM`, `git.exe`).Run()
		mu.Unlock()
	}
	for _, path := range tg.temps {
		tg.check(os.RemoveAll(path))
	}
	if tg.tempdir != "" {
		tg.check(os.RemoveAll(tg.tempdir))
	}
}

// readManifest returns the manifest in the current directory.
func (tg *testgoData) readManifest() string {
	m := filepath.Join(tg.pwd(), "manifest.json")
	tg.mustExist(m)

	f, err := ioutil.ReadFile(m)
	tg.must(err)
	return string(f)
}

// readLock returns the lock in the current directory.
func (tg *testgoData) readLock() string {
	l := filepath.Join(tg.pwd(), "lock.json")
	tg.mustExist(l)

	f, err := ioutil.ReadFile(l)
	tg.must(err)
	return string(f)
}

func (tg *testgoData) getCommit(repo string) string {
	repoPath := tg.path("pkg/hoard/sources/https---" + strings.Replace(repo, "/", "-", -1))
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		tg.t.Fatalf("git commit failed: out -> %s err -> %v", string(out), err)
	}
	return strings.TrimSpace(string(out))
}
