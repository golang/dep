// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"path/filepath"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/test"
)

func TestGetDirectDependencies_ConsolidatesRootProjects(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	ctx := NewTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	testprj := "directdepstest"
	testdir := filepath.Join("src", testprj)
	h.TempDir(testdir)
	h.TempCopy(filepath.Join(testdir, "main.go"), "init/directdeps/main.go")

	testpath := h.Path(testdir)
	prj := &dep.Project{AbsRoot: testpath, ResolvedAbsRoot: testpath, ImportRoot: gps.ProjectRoot(testprj)}

	_, dd, err := getDirectDependencies(sm, prj)
	h.Must(err)

	wantpr := "github.com/carolynvs/deptest-subpkg"
	if _, has := dd[wantpr]; !has {
		t.Fatalf("Expected direct dependencies to contain %s, got %v", wantpr, dd)
	}
}
