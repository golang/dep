// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/errors"
)

var (
	// ExeSuffix is the suffix of executable files; ".exe" on Windows.
	ExeSuffix string
	mu        sync.Mutex
	// PrintLogs controls logging of test commands.
	PrintLogs = flag.Bool("logs", false, "log stdin/stdout of test commands")
	// UpdateGolden controls updating test fixtures.
	UpdateGolden = flag.Bool("update", false, "update golden files")
)

const (
	manifestName = "Gopkg.toml"
	lockName     = "Gopkg.lock"
)

func init() {
	switch runtime.GOOS {
	case "windows":
		ExeSuffix = ".exe"
	}
}

// Helper with utilities for testing.
type Helper struct {
	t              *testing.T
	temps          []string
	wd             string
	origWd         string
	env            []string
	tempdir        string
	ran            bool
	inParallel     bool
	stdout, stderr bytes.Buffer
}

// NewHelper initializes a new helper for testing.
func NewHelper(t *testing.T) *Helper {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return &Helper{t: t, origWd: wd}
}

// Must gives a fatal error if err is not nil.
func (h *Helper) Must(err error) {
	if err != nil {
		h.t.Fatalf("%+v", err)
	}
}

// check gives a test non-fatal error if err is not nil.
func (h *Helper) check(err error) {
	if err != nil {
		h.t.Errorf("%+v", err)
	}
}

// Parallel runs the test in parallel by calling t.Parallel.
func (h *Helper) Parallel() {
	if h.ran {
		h.t.Fatalf("%+v", errors.New("internal testsuite error: call to parallel after run"))
	}
	if h.wd != "" {
		h.t.Fatalf("%+v", errors.New("internal testsuite error: call to parallel after cd"))
	}
	for _, e := range h.env {
		if strings.HasPrefix(e, "GOROOT=") || strings.HasPrefix(e, "GOPATH=") || strings.HasPrefix(e, "GOBIN=") {
			val := e[strings.Index(e, "=")+1:]
			if strings.HasPrefix(val, "testdata") || strings.HasPrefix(val, "./testdata") {
				h.t.Fatalf("%+v", errors.Errorf("internal testsuite error: call to parallel with testdata in environment (%s)", e))
			}
		}
	}
	h.inParallel = true
	h.t.Parallel()
}

// pwd returns the current directory.
func (h *Helper) pwd() string {
	wd, err := os.Getwd()
	if err != nil {
		h.t.Fatalf("%+v", errors.Wrap(err, "could not get working directory"))
	}
	return wd
}

// Cd changes the current directory to the named directory. Note that
// using this means that the test must not be run in parallel with any
// other tests.
func (h *Helper) Cd(dir string) {
	if h.inParallel {
		h.t.Fatalf("%+v", errors.New("internal testsuite error: changing directory when running in parallel"))
	}
	if h.wd == "" {
		h.wd = h.pwd()
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		h.Setenv("PWD", abs)
	}

	err = os.Chdir(dir)
	h.Must(errors.Wrapf(err, "Unable to cd to %s", dir))
}

// Setenv sets an environment variable to use when running the test go
// command.
func (h *Helper) Setenv(name, val string) {
	if h.inParallel && (name == "GOROOT" || name == "GOPATH" || name == "GOBIN") && (strings.HasPrefix(val, "testdata") || strings.HasPrefix(val, "./testdata")) {
		h.t.Fatalf("%+v", errors.Errorf("internal testsuite error: call to setenv with testdata (%s=%s) after parallel", name, val))
	}
	h.unsetenv(name)
	h.env = append(h.env, name+"="+val)
}

// unsetenv removes an environment variable.
func (h *Helper) unsetenv(name string) {
	if h.env == nil {
		h.env = append([]string(nil), os.Environ()...)
	}
	for i, v := range h.env {
		if strings.HasPrefix(v, name+"=") {
			h.env = append(h.env[:i], h.env[i+1:]...)
			break
		}
	}
}

// DoRun runs the test go command, recording stdout and stderr and
// returning exit status.
func (h *Helper) DoRun(args []string) error {
	if h.inParallel {
		for _, arg := range args {
			if strings.HasPrefix(arg, "testdata") || strings.HasPrefix(arg, "./testdata") {
				h.t.Fatalf("%+v", errors.New("internal testsuite error: parallel run using testdata"))
			}
		}
	}
	if *PrintLogs {
		h.t.Logf("running testdep %v", args)
	}
	var prog string
	if h.wd == "" {
		prog = "./testdep" + ExeSuffix
	} else {
		prog = filepath.Join(h.wd, "testdep"+ExeSuffix)
	}
	newargs := args
	if args[0] != "check" {
		newargs = append([]string{args[0], "-v"}, args[1:]...)
	}

	cmd := exec.Command(prog, newargs...)
	h.stdout.Reset()
	h.stderr.Reset()
	cmd.Stdout = &h.stdout
	cmd.Stderr = &h.stderr
	cmd.Env = h.env
	status := cmd.Run()
	if *PrintLogs {
		if h.stdout.Len() > 0 {
			h.t.Log("standard output:")
			h.t.Log(h.stdout.String())
		}
		if h.stderr.Len() > 0 {
			h.t.Log("standard error:")
			h.t.Log(h.stderr.String())
		}
	}
	h.ran = true
	return errors.Wrapf(status, "Error running %s\n%s", strings.Join(newargs, " "), h.stderr.String())
}

// Run runs the test go command, and expects it to succeed.
func (h *Helper) Run(args ...string) {
	if runtime.GOOS == "windows" {
		mu.Lock()
		defer mu.Unlock()
	}
	if status := h.DoRun(args); status != nil {
		h.t.Logf("go %v failed unexpectedly: %v", args, status)
		h.t.FailNow()
	}
}

// runFail runs the test go command, and expects it to fail.
func (h *Helper) runFail(args ...string) {
	if status := h.DoRun(args); status == nil {
		h.t.Fatalf("%+v", errors.New("testgo succeeded unexpectedly"))
	} else {
		h.t.Log("testgo failed as expected:", status)
	}
}

// RunGo runs a go command, and expects it to succeed.
func (h *Helper) RunGo(args ...string) {
	cmd := exec.Command("go", args...)
	h.stdout.Reset()
	h.stderr.Reset()
	cmd.Stdout = &h.stdout
	cmd.Stderr = &h.stderr
	cmd.Dir = h.wd
	cmd.Env = h.env
	status := cmd.Run()
	if h.stdout.Len() > 0 {
		h.t.Log("go standard output:")
		h.t.Log(h.stdout.String())
	}
	if h.stderr.Len() > 0 {
		h.t.Log("go standard error:")
		h.t.Log(h.stderr.String())
	}
	if status != nil {
		h.t.Logf("go %v failed unexpectedly: %v", args, status)
		h.t.FailNow()
	}
}

// NeedsExternalNetwork makes sure the tests needing external network will not
// be run when executing tests in short mode.
func NeedsExternalNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test: no external network in -short mode")
	}
}

// NeedsGit will make sure the tests that require git will be skipped if the
// git binary is not available.
func NeedsGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skipping because git binary not found")
	}
}

// RunGit runs a git command, and expects it to succeed.
func (h *Helper) RunGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	h.stdout.Reset()
	h.stderr.Reset()
	cmd.Stdout = &h.stdout
	cmd.Stderr = &h.stderr
	cmd.Dir = dir
	cmd.Env = h.env
	status := cmd.Run()
	if *PrintLogs {
		if h.stdout.Len() > 0 {
			h.t.Logf("git %v standard output:", args)
			h.t.Log(h.stdout.String())
		}
		if h.stderr.Len() > 0 {
			h.t.Logf("git %v standard error:", args)
			h.t.Log(h.stderr.String())
		}
	}
	if status != nil {
		h.t.Logf("git %v failed unexpectedly: %v", args, status)
		h.t.FailNow()
	}
}

// getStdout returns standard output of the testgo run as a string.
func (h *Helper) getStdout() string {
	if !h.ran {
		h.t.Fatalf("%+v", errors.New("internal testsuite error: stdout called before run"))
	}
	return h.stdout.String()
}

// getStderr returns standard error of the testgo run as a string.
func (h *Helper) getStderr() string {
	if !h.ran {
		h.t.Fatalf("%+v", errors.New("internal testsuite error: stdout called before run"))
	}
	return h.stderr.String()
}

// doGrepMatch looks for a regular expression in a buffer, and returns
// whether it is found. The regular expression is matched against
// each line separately, as with the grep command.
func (h *Helper) doGrepMatch(match string, b *bytes.Buffer) bool {
	if !h.ran {
		h.t.Fatalf("%+v", errors.New("internal testsuite error: grep called before run"))
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
func (h *Helper) doGrep(match string, b *bytes.Buffer, name, msg string) {
	if !h.doGrepMatch(match, b) {
		h.t.Log(msg)
		h.t.Logf("pattern %v not found in standard %s", match, name)
		h.t.FailNow()
	}
}

// grepStdout looks for a regular expression in the test run's
// standard output and fails, logging msg, if it is not found.
func (h *Helper) grepStdout(match, msg string) {
	h.doGrep(match, &h.stdout, "output", msg)
}

// grepStderr looks for a regular expression in the test run's
// standard error and fails, logging msg, if it is not found.
func (h *Helper) grepStderr(match, msg string) {
	h.doGrep(match, &h.stderr, "error", msg)
}

// grepBoth looks for a regular expression in the test run's standard
// output or stand error and fails, logging msg, if it is not found.
func (h *Helper) grepBoth(match, msg string) {
	if !h.doGrepMatch(match, &h.stdout) && !h.doGrepMatch(match, &h.stderr) {
		h.t.Log(msg)
		h.t.Logf("pattern %v not found in standard output or standard error", match)
		h.t.FailNow()
	}
}

// doGrepNot looks for a regular expression in a buffer and fails if
// it is found. The name and msg arguments are as for doGrep.
func (h *Helper) doGrepNot(match string, b *bytes.Buffer, name, msg string) {
	if h.doGrepMatch(match, b) {
		h.t.Log(msg)
		h.t.Logf("pattern %v found unexpectedly in standard %s", match, name)
		h.t.FailNow()
	}
}

// grepStdoutNot looks for a regular expression in the test run's
// standard output and fails, logging msg, if it is found.
func (h *Helper) grepStdoutNot(match, msg string) {
	h.doGrepNot(match, &h.stdout, "output", msg)
}

// grepStderrNot looks for a regular expression in the test run's
// standard error and fails, logging msg, if it is found.
func (h *Helper) grepStderrNot(match, msg string) {
	h.doGrepNot(match, &h.stderr, "error", msg)
}

// grepBothNot looks for a regular expression in the test run's
// standard output or stand error and fails, logging msg, if it is
// found.
func (h *Helper) grepBothNot(match, msg string) {
	if h.doGrepMatch(match, &h.stdout) || h.doGrepMatch(match, &h.stderr) {
		h.t.Log(msg)
		h.t.Fatalf("%+v", errors.Errorf("pattern %v found unexpectedly in standard output or standard error", match))
	}
}

// doGrepCount counts the number of times a regexp is seen in a buffer.
func (h *Helper) doGrepCount(match string, b *bytes.Buffer) int {
	if !h.ran {
		h.t.Fatalf("%+v", errors.New("internal testsuite error: doGrepCount called before run"))
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
func (h *Helper) grepCountBoth(match string) int {
	return h.doGrepCount(match, &h.stdout) + h.doGrepCount(match, &h.stderr)
}

// creatingTemp records that the test plans to create a temporary file
// or directory. If the file or directory exists already, it will be
// removed. When the test completes, the file or directory will be
// removed if it exists.
func (h *Helper) creatingTemp(path string) {
	if filepath.IsAbs(path) && !strings.HasPrefix(path, h.tempdir) {
		h.t.Fatalf("%+v", errors.Errorf("internal testsuite error: creatingTemp(%q) with absolute path not in temporary directory", path))
	}
	// If we have changed the working directory, make sure we have
	// an absolute path, because we are going to change directory
	// back before we remove the temporary.
	if h.wd != "" && !filepath.IsAbs(path) {
		path = filepath.Join(h.pwd(), path)
	}
	h.Must(os.RemoveAll(path))
	h.temps = append(h.temps, path)
}

// makeTempdir makes a temporary directory for a run of testgo. If
// the temporary directory was already created, this does nothing.
func (h *Helper) makeTempdir() {
	if h.tempdir == "" {
		var err error
		h.tempdir, err = ioutil.TempDir("", "gotest")
		h.Must(err)
	}
}

// TempFile adds a temporary file for a run of testgo.
func (h *Helper) TempFile(path, contents string) {
	h.makeTempdir()
	h.Must(os.MkdirAll(filepath.Join(h.tempdir, filepath.Dir(path)), 0755))
	bytes := []byte(contents)
	if strings.HasSuffix(path, ".go") {
		formatted, err := format.Source(bytes)
		if err == nil {
			bytes = formatted
		}
	}
	h.Must(ioutil.WriteFile(filepath.Join(h.tempdir, path), bytes, 0644))
}

// WriteTestFile writes a file to the testdata directory from memory.  src is
// relative to ./testdata.
func (h *Helper) WriteTestFile(src string, content string) error {
	err := ioutil.WriteFile(filepath.Join(h.origWd, "testdata", src), []byte(content), 0666)
	return err
}

// GetFile reads a file into memory
func (h *Helper) GetFile(path string) io.ReadCloser {
	content, err := os.Open(path)
	if err != nil {
		h.t.Fatalf("%+v", errors.Wrapf(err, "Unable to open file: %s", path))
	}
	return content
}

// GetTestFile reads a file from the testdata directory into memory.  src is
// relative to ./testdata.
func (h *Helper) GetTestFile(src string) io.ReadCloser {
	fullPath := filepath.Join(h.origWd, "testdata", src)
	return h.GetFile(fullPath)
}

// GetTestFileString reads a file from the testdata directory into memory.  src is
// relative to ./testdata.
func (h *Helper) GetTestFileString(src string) string {
	srcf := h.GetTestFile(src)
	defer srcf.Close()
	content, err := ioutil.ReadAll(srcf)
	if err != nil {
		h.t.Fatalf("%+v", err)
	}
	return string(content)
}

// TempCopy copies a temporary file from testdata into the temporary directory.
// dest is relative to the temp directory location, and src is relative to
// ./testdata.
func (h *Helper) TempCopy(dest, src string) {
	in := h.GetTestFile(src)
	defer in.Close()
	h.TempDir(filepath.Dir(dest))
	out, err := os.Create(filepath.Join(h.tempdir, dest))
	if err != nil {
		panic(err)
	}
	defer out.Close()
	io.Copy(out, in)
}

// TempDir adds a temporary directory for a run of testgo.
func (h *Helper) TempDir(path string) {
	h.makeTempdir()
	fullPath := filepath.Join(h.tempdir, path)
	if err := os.MkdirAll(fullPath, 0755); err != nil && !os.IsExist(err) {
		h.t.Fatalf("%+v", errors.Errorf("Unable to create temp directory: %s", fullPath))
	}
}

// Path returns the absolute pathname to file with the temporary
// directory.
func (h *Helper) Path(name string) string {
	if h.tempdir == "" {
		h.t.Fatalf("%+v", errors.Errorf("internal testsuite error: path(%q) with no tempdir", name))
	}

	var joined string
	if name == "." {
		joined = h.tempdir
	} else {
		joined = filepath.Join(h.tempdir, name)
	}

	// Ensure it's the absolute, symlink-less path we're returning
	abs, err := filepath.EvalSymlinks(joined)
	if err != nil {
		h.t.Fatalf("%+v", errors.Wrapf(err, "internal testsuite error: could not get absolute path for dir(%q)", joined))
	}
	return abs
}

// MustExist fails if path does not exist.
func (h *Helper) MustExist(path string) {
	if err := h.ShouldExist(path); err != nil {
		h.t.Fatalf("%+v", err)
	}
}

// ShouldExist returns an error if path does not exist.
func (h *Helper) ShouldExist(path string) error {
	if !h.Exist(path) {
		return errors.Errorf("%s does not exist but should", path)
	}

	return nil
}

// Exist returns whether or not a path exists
func (h *Helper) Exist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		h.t.Fatalf("%+v", errors.Wrapf(err, "Error checking if path exists: %s", path))
	}

	return true
}

// MustNotExist fails if path exists.
func (h *Helper) MustNotExist(path string) {
	if err := h.ShouldNotExist(path); err != nil {
		h.t.Fatalf("%+v", err)
	}
}

// ShouldNotExist returns an error if path exists.
func (h *Helper) ShouldNotExist(path string) error {
	if h.Exist(path) {
		return errors.Errorf("%s exists but should not", path)
	}

	return nil
}

// Cleanup cleans up a test that runs testgo.
func (h *Helper) Cleanup() {
	if h.wd != "" {
		if err := os.Chdir(h.wd); err != nil {
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
	for _, path := range h.temps {
		h.check(os.RemoveAll(path))
	}
	if h.tempdir != "" {
		h.check(os.RemoveAll(h.tempdir))
	}
}

// ReadManifest returns the manifest in the current directory.
func (h *Helper) ReadManifest() string {
	m := filepath.Join(h.pwd(), manifestName)
	h.MustExist(m)

	f, err := ioutil.ReadFile(m)
	h.Must(err)
	return string(f)
}

// ReadLock returns the lock in the current directory.
func (h *Helper) ReadLock() string {
	l := filepath.Join(h.pwd(), lockName)
	h.MustExist(l)

	f, err := ioutil.ReadFile(l)
	h.Must(err)
	return string(f)
}

// GetCommit treats repo as a path to a git repository and returns the current
// revision.
func (h *Helper) GetCommit(repo string) string {
	repoPath := h.Path("pkg/dep/sources/https---" + strings.Replace(repo, "/", "-", -1))
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("%+v", errors.Wrapf(err, "git commit failed: out -> %s", string(out)))
	}
	return strings.TrimSpace(string(out))
}
