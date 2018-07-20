// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

const (
	projectRoot = "src/github.com/golang/notexist"
)

// RunFunc defines the function signature for an integration test command to execute.
type RunFunc func(prog string, newargs []string, outW, errW io.Writer, dir string, env []string) error

// TestProject manages the "virtual" test project directory structure
// and content
type TestProject struct {
	t          *testing.T
	preImports []string
	tempdir    string
	env        []string
	origWd     string
	stdout     bytes.Buffer
	stderr     bytes.Buffer
	run        RunFunc
}

// NewTestProject initializes a new test's project directory.
func NewTestProject(t *testing.T, initPath, wd string, run RunFunc) *TestProject {
	// Cleaning up the GIT_DIR variable is useful when running tests under git
	// rebase. In any case, since we're operating with temporary clones,
	// no pre-existing value could be useful here.
	// We do it globally because the internal runs don't actually use the
	// TestProject's environment.
	os.Unsetenv("GIT_DIR")

	new := &TestProject{
		t:      t,
		origWd: wd,
		env:    os.Environ(),
		run:    run,
	}
	new.makeRootTempDir()
	new.TempDir(projectRoot, "vendor")
	new.CopyTree(initPath)

	new.Setenv("GOPATH", new.tempdir)

	return new
}

// Cleanup (remove) the test project's directory.
func (p *TestProject) Cleanup() {
	os.RemoveAll(p.tempdir)
}

// Path to the test project directory.
func (p *TestProject) Path(args ...string) string {
	return filepath.Join(p.tempdir, filepath.Join(args...))
}

// ProjPath builds an import path for the test project.
func (p *TestProject) ProjPath(args ...string) string {
	localPath := append([]string{projectRoot}, args...)
	return p.Path(localPath...)
}

// TempDir creates a temporary directory for the test project.
func (p *TestProject) TempDir(args ...string) {
	fullPath := p.Path(args...)
	if err := os.MkdirAll(fullPath, 0755); err != nil && !os.IsExist(err) {
		p.t.Fatalf("%+v", errors.Errorf("Unable to create temp directory: %s", fullPath))
	}
}

// TempProjDir builds the path to a package within the test project.
func (p *TestProject) TempProjDir(args ...string) {
	localPath := append([]string{projectRoot}, args...)
	p.TempDir(localPath...)
}

// VendorPath lists the contents of the test project's vendor directory.
func (p *TestProject) VendorPath(args ...string) string {
	localPath := append([]string{projectRoot, "vendor"}, args...)
	p.TempDir(localPath...)
	return p.Path(localPath...)
}

// RunGo runs a go command, and expects it to succeed.
func (p *TestProject) RunGo(args ...string) {
	cmd := exec.Command("go", args...)
	p.stdout.Reset()
	p.stderr.Reset()
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	cmd.Dir = p.tempdir
	cmd.Env = p.env
	status := cmd.Run()
	if p.stdout.Len() > 0 {
		p.t.Log("go standard output:")
		p.t.Log(p.stdout.String())
	}
	if p.stderr.Len() > 0 {
		p.t.Log("go standard error:")
		p.t.Log(p.stderr.String())
	}
	if status != nil {
		p.t.Logf("go %v failed unexpectedly: %v", args, status)
		p.t.FailNow()
	}
}

// RunGit runs a git command, and expects it to succeed.
func (p *TestProject) RunGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	p.stdout.Reset()
	p.stderr.Reset()
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	cmd.Dir = dir
	cmd.Env = p.env
	status := cmd.Run()
	if *test.PrintLogs {
		if p.stdout.Len() > 0 {
			p.t.Logf("git %v standard output:", args)
			p.t.Log(p.stdout.String())
		}
		if p.stderr.Len() > 0 {
			p.t.Logf("git %v standard error:", args)
			p.t.Log(p.stderr.String())
		}
	}
	if status != nil {
		p.t.Logf("git %v failed unexpectedly: %v", args, status)
		p.t.FailNow()
	}
}

// GetStdout gets the Stdout output from test run.
func (p *TestProject) GetStdout() string {
	return p.stdout.String()
}

// GetStderr gets the Stderr output from test run.
func (p *TestProject) GetStderr() string {
	return p.stderr.String()
}

// GetVendorGit populates the initial vendor directory for a test project.
func (p *TestProject) GetVendorGit(ip string) {
	parse := strings.Split(ip, "/")
	gitDir := strings.Join(parse[:len(parse)-1], string(filepath.Separator))
	p.TempProjDir("vendor", gitDir)
	p.RunGit(p.ProjPath("vendor", gitDir), "clone", "http://"+ip)
}

// DoRun executes the integration test command against the test project.
func (p *TestProject) DoRun(args []string) error {
	if *test.PrintLogs {
		p.t.Logf("running testdep %v", args)
	}
	prog := filepath.Join(p.origWd, "testdep"+test.ExeSuffix)

	newargs := args
	if args[0] != "check" {
		newargs = append([]string{args[0], "-v"}, args[1:]...)
	}

	p.stdout.Reset()
	p.stderr.Reset()

	status := p.run(prog, newargs, &p.stdout, &p.stderr, p.ProjPath(""), p.env)

	if *test.PrintLogs {
		if p.stdout.Len() > 0 {
			p.t.Logf("\nstandard output:%s", p.stdout.String())
		}
		if p.stderr.Len() > 0 {
			p.t.Logf("standard error:\n%s", p.stderr.String())
		}
	}
	return status
}

// CopyTree recursively copies a source directory into the test project's directory.
func (p *TestProject) CopyTree(src string) {
	filepath.Walk(src,
		func(path string, info os.FileInfo, err error) error {
			if path != src {
				localpath := path[len(src)+1:]
				if info.IsDir() {
					p.TempDir(projectRoot, localpath)
				} else {
					destpath := filepath.Join(p.ProjPath(), localpath)
					copyFile(destpath, path)
				}
			}
			return nil
		})
}

func copyFile(dest, src string) {
	in, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	io.Copy(out, in)
}

// GetVendorPaths collects final vendor paths at a depth of three levels.
func (p *TestProject) GetVendorPaths() []string {
	vendorPath := p.ProjPath("vendor")
	result := make([]string, 0)
	filepath.Walk(
		vendorPath,
		func(path string, info os.FileInfo, err error) error {
			if len(path) > len(vendorPath) && info.IsDir() {
				parse := strings.Split(path[len(vendorPath)+1:], string(filepath.Separator))
				if len(parse) == 3 {
					result = append(result, strings.Join(parse, "/"))
					return filepath.SkipDir
				}
			}
			return nil
		},
	)
	sort.Strings(result)
	return result
}

// GetImportPaths collect final vendor paths at a depth of three levels.
func (p *TestProject) GetImportPaths() []string {
	importPath := p.Path("src")
	result := make([]string, 0)
	filepath.Walk(
		importPath,
		func(path string, info os.FileInfo, err error) error {
			if len(path) > len(importPath) && info.IsDir() {
				parse := strings.Split(path[len(importPath)+1:], string(filepath.Separator))
				if len(parse) == 3 {
					result = append(result, strings.Join(parse, "/"))
					return filepath.SkipDir
				}
			}
			return nil
		},
	)
	sort.Strings(result)
	return result
}

// RecordImportPaths takes a snapshot of the import paths before test is run.
func (p *TestProject) RecordImportPaths() {
	p.preImports = p.GetImportPaths()
}

// CompareImportPaths compares import paths before and after test commands.
func (p *TestProject) CompareImportPaths() {
	wantImportPaths := p.preImports
	gotImportPaths := p.GetImportPaths()
	if len(gotImportPaths) != len(wantImportPaths) {
		p.t.Fatalf("Import path count changed during command: pre %d post %d", len(wantImportPaths), len(gotImportPaths))
	}
	for ind := range gotImportPaths {
		if gotImportPaths[ind] != wantImportPaths[ind] {
			p.t.Errorf("Change in import paths during: pre %s post %s", gotImportPaths, wantImportPaths)
		}
	}
}

// makeRootTempdir makes a temporary directory for a run of testgo. If
// the temporary directory was already created, this does nothing.
func (p *TestProject) makeRootTempDir() {
	if p.tempdir == "" {
		var err error
		p.tempdir, err = ioutil.TempDir("", "gotest")
		p.Must(err)

		// Fix for OSX where the tempdir is a symlink:
		if runtime.GOOS == "darwin" {
			p.tempdir, err = filepath.EvalSymlinks(p.tempdir)
			p.Must(err)
		}
	}
}

// Setenv sets an environment variable to use when running the test go
// command.
func (p *TestProject) Setenv(name, val string) {
	p.env = append(p.env, name+"="+val)
}

// Must gives a fatal error if err is not nil.
func (p *TestProject) Must(err error) {
	if err != nil {
		p.t.Fatalf("%+v", err)
	}
}
