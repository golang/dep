// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

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

	"github.com/pkg/errors"
)

const (
	ProjectRoot string = "src/github.com/golang/notexist"
)

type RunFunc func(prog string, newargs []string, outW, errW io.Writer, dir string, env []string) error

// IntegrationTestProject manages the "virtual" test project directory structure
// and content
type IntegrationTestProject struct {
	t          *testing.T
	h          *Helper
	preImports []string
	tempdir    string
	env        []string
	origWd     string
	stdout     bytes.Buffer
	stderr     bytes.Buffer
	run        RunFunc
}

func NewTestProject(t *testing.T, initPath, wd string, externalProc bool, run RunFunc) *IntegrationTestProject {
	new := &IntegrationTestProject{
		t:      t,
		origWd: wd,
		env:    os.Environ(),
		run:    run,
	}
	new.makeRootTempDir()
	new.TempDir(ProjectRoot, "vendor")
	new.CopyTree(initPath)

	// Note that the Travis darwin platform, directories with certain roots such
	// as /var are actually links to a dirtree under /private.  Without the patch
	// below the wd, and therefore the GOPATH, is recorded as "/var/..." but the
	// actual process runs in "/private/var/..." and dies due to not being in the
	// GOPATH because the roots don't line up.
	if externalProc && runtime.GOOS == "darwin" && needsPrivateLeader(new.tempdir) {
		new.Setenv("GOPATH", filepath.Join("/private", new.tempdir))
	} else {
		new.Setenv("GOPATH", new.tempdir)
	}

	return new
}

func (p *IntegrationTestProject) Cleanup() {
	os.RemoveAll(p.tempdir)
}

func (p *IntegrationTestProject) Path(args ...string) string {
	return filepath.Join(p.tempdir, filepath.Join(args...))
}

func (p *IntegrationTestProject) ProjPath(args ...string) string {
	localPath := append([]string{ProjectRoot}, args...)
	return p.Path(localPath...)
}

func (p *IntegrationTestProject) TempDir(args ...string) {
	fullPath := p.Path(args...)
	if err := os.MkdirAll(fullPath, 0755); err != nil && !os.IsExist(err) {
		p.t.Fatalf("%+v", errors.Errorf("Unable to create temp directory: %s", fullPath))
	}
}

func (p *IntegrationTestProject) TempProjDir(args ...string) {
	localPath := append([]string{ProjectRoot}, args...)
	p.TempDir(localPath...)
}

func (p *IntegrationTestProject) VendorPath(args ...string) string {
	localPath := append([]string{ProjectRoot, "vendor"}, args...)
	p.TempDir(localPath...)
	return p.Path(localPath...)
}

func (p *IntegrationTestProject) RunGo(args ...string) {
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

func (p *IntegrationTestProject) RunGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	p.stdout.Reset()
	p.stderr.Reset()
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	cmd.Dir = dir
	cmd.Env = p.env
	status := cmd.Run()
	if *PrintLogs {
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

// GetStdout gets the Stdout output from test run
func (p *IntegrationTestProject) GetStdout() string {
	return p.stdout.String()
}

// GetStderr gets the Stderr output from test run
func (p *IntegrationTestProject) GetStderr() string {
	return p.stderr.String()
}

func (p *IntegrationTestProject) GetVendorGit(ip string) {
	parse := strings.Split(ip, "/")
	gitDir := strings.Join(parse[:len(parse)-1], string(filepath.Separator))
	p.TempProjDir("vendor", gitDir)
	p.RunGit(p.ProjPath("vendor", gitDir), "clone", "http://"+ip)
}

func (p *IntegrationTestProject) DoRun(args []string) error {
	if *PrintLogs {
		p.t.Logf("running testdep %v", args)
	}
	prog := filepath.Join(p.origWd, "testdep"+ExeSuffix)
	newargs := append([]string{args[0], "-v"}, args[1:]...)

	p.stdout.Reset()
	p.stderr.Reset()

	status := p.run(prog, newargs, &p.stdout, &p.stderr, p.ProjPath(""), p.env)

	if *PrintLogs {
		if p.stdout.Len() > 0 {
			p.t.Logf("\nstandard output:%s", p.stdout.String())
		}
		if p.stderr.Len() > 0 {
			p.t.Logf("standard error:\n%s", p.stderr.String())
		}
	}
	return status
}

func (p *IntegrationTestProject) CopyTree(src string) {
	filepath.Walk(src,
		func(path string, info os.FileInfo, err error) error {
			if path != src {
				localpath := path[len(src)+1:]
				if info.IsDir() {
					p.TempDir(ProjectRoot, localpath)
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

// Collect final vendor paths at a depth of three levels
func (p *IntegrationTestProject) GetVendorPaths() []string {
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

// Collect final vendor paths at a depth of three levels
func (p *IntegrationTestProject) GetImportPaths() []string {
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

// Take a snapshot of the import paths before test is run
func (p *IntegrationTestProject) RecordImportPaths() {
	p.preImports = p.GetImportPaths()
}

// Compare import paths before and after commands
func (p *IntegrationTestProject) CompareImportPaths() {
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
func (p *IntegrationTestProject) makeRootTempDir() {
	if p.tempdir == "" {
		var err error
		p.tempdir, err = ioutil.TempDir("", "gotest")
		p.Must(err)
	}
}

// Setenv sets an environment variable to use when running the test go
// command.
func (p *IntegrationTestProject) Setenv(name, val string) {
	p.env = append(p.env, name+"="+val)
}

// Must gives a fatal error if err is not nil.
func (p *IntegrationTestProject) Must(err error) {
	if err != nil {
		p.t.Fatalf("%+v", err)
	}
}

// Checks for filepath beginnings that result in the "/private" leader
// on Mac platforms
func needsPrivateLeader(path string) bool {
	var roots = []string{"/var", "/tmp", "/etc"}
	for _, root := range roots {
		if strings.HasPrefix(path, root) {
			return true
		}
	}
	return false
}
