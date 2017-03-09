// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	ProjectRoot string = "src/github.com/golang/notexist"
)

// IntegrationTestProject manages the "virtual" test project directory structure
// and content
type IntegrationTestProject struct {
	t          *testing.T
	h          *Helper
	preImports []string
}

func NewTestProject(t *testing.T, initPath string) *IntegrationTestProject {
	new := &IntegrationTestProject{
		t: t,
		h: NewHelper(t),
	}
	new.TempDir(ProjectRoot)
	new.TempDir(ProjectRoot, "vendor")
	new.CopyTree(initPath)
	new.h.Setenv("GOPATH", new.h.Path("."))
	new.h.Cd(new.Path(ProjectRoot))
	return new
}

func (p *IntegrationTestProject) Cleanup() {
	p.h.Cleanup()
}

func (p *IntegrationTestProject) Path(args ...string) string {
	return p.h.Path(filepath.Join(args...))
}

func (p *IntegrationTestProject) TempDir(args ...string) {
	p.h.TempDir(filepath.Join(args...))
}

func (p *IntegrationTestProject) TempProjDir(args ...string) {
	localPath := append([]string{ProjectRoot}, args...)
	p.h.TempDir(filepath.Join(localPath...))
}

func (p *IntegrationTestProject) ProjPath(args ...string) string {
	localPath := append([]string{ProjectRoot}, args...)
	return p.Path(localPath...)
}

func (p *IntegrationTestProject) VendorPath(args ...string) string {
	localPath := append([]string{ProjectRoot, "vendor"}, args...)
	p.TempDir(localPath...)
	return p.Path(localPath...)
}

func (p *IntegrationTestProject) RunGo(args ...string) {
	p.h.RunGo(args...)
}

func (p *IntegrationTestProject) RunGit(dir string, args ...string) {
	p.h.RunGit(dir, args...)
}

func (p *IntegrationTestProject) GetVendorGit(ip string) {
	parse := strings.Split(ip, "/")
	gitDir := strings.Join(parse[:len(parse)-1], string(filepath.Separator))
	p.TempProjDir("vendor", gitDir)
	p.RunGit(p.ProjPath("vendor", gitDir), "clone", "http://"+ip)
}

func (p *IntegrationTestProject) DoRun(args []string) error {
	return p.h.DoRun(args)
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
