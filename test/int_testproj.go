// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var (
	ProjectRoot string = "src/github.com/golang/notexist"
)

// To manage a test case directory structure and content
type IntegrationTestProject struct {
	t *testing.T
	h *Helper
}

func NewTestProject(t *testing.T, initPath string) *IntegrationTestProject {
	new := &IntegrationTestProject{
		t: t,
		h: NewHelper(t),
	}
	new.h.TempDir("src")
	new.h.Setenv("GOPATH", new.h.Path("."))
	new.h.TempCopyTree(ProjectRoot, initPath)
	new.h.Cd(new.Path(ProjectRoot))
	return new
}

func (p *IntegrationTestProject) Cleanup() {
	p.h.Cleanup()
}

func (p *IntegrationTestProject) Path(args ...string) string {
	return p.h.Path(filepath.Join(args...))
}

func (p *IntegrationTestProject) ProjPath(args ...string) string {
	localPath := append([]string{ProjectRoot}, args...)
	return p.Path(localPath...)
}

func (p *IntegrationTestProject) RunGo(args ...string) {
	p.h.RunGo(args...)
}

func (p *IntegrationTestProject) RunGit(dir string, args ...string) {
	p.h.RunGit(dir, args...)
}

func (p *IntegrationTestProject) DoRun(args []string) error {
	return p.h.DoRun(args)
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
