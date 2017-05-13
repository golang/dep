// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/pkgtree"
	fb "github.com/golang/dep/internal/feedback"
)

// gopathAnalyzer deduces configuration from the projects in the GOPATH
type gopathAnalyzer struct {
	ctx  *dep.Ctx
	pkgT pkgtree.PackageTree
	cpr  string
	sm   *gps.SourceMgr

	pd    projectData
	origL *dep.Lock
}

func newGopathAnalyzer(ctx *dep.Ctx, pkgT pkgtree.PackageTree, cpr string, sm *gps.SourceMgr) *gopathAnalyzer {
	return &gopathAnalyzer{
		ctx:  ctx,
		pkgT: pkgT,
		cpr:  cpr,
		sm:   sm,
	}
}

func (a *gopathAnalyzer) DeriveRootManifestAndLock(path string, n gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	var err error

	a.pd, err = getProjectData(a.ctx, a.pkgT, a.cpr, a.sm)
	if err != nil {
		return nil, nil, err
	}
	m := &dep.Manifest{
		Dependencies: a.pd.constraints,
	}

	// Make an initial lock from what knowledge we've collected about the
	// versions on disk
	l := &dep.Lock{
		P: make([]gps.LockedProject, 0, len(a.pd.ondisk)),
	}

	for pr, v := range a.pd.ondisk {
		// That we have to chop off these path prefixes is a symptom of
		// a problem in gps itself
		pkgs := make([]string, 0, len(a.pd.dependencies[pr]))
		prslash := string(pr) + "/"
		for _, pkg := range a.pd.dependencies[pr] {
			if pkg == string(pr) {
				pkgs = append(pkgs, ".")
			} else {
				pkgs = append(pkgs, trimPathPrefix(pkg, prslash))
			}
		}

		l.P = append(l.P, gps.NewLockedProject(
			gps.ProjectIdentifier{ProjectRoot: pr}, v, pkgs),
		)
	}

	// Copy lock before solving. Use this to separate new lock projects from soln
	a.origL = l

	return m, l, nil
}

func (a *gopathAnalyzer) PostSolveShenanigans(m *dep.Manifest, l *dep.Lock) {
	// Iterate through the new projects in solved lock and add them to manifest
	// if direct deps and log feedback for all the new projects.
	for _, x := range l.Projects() {
		pr := x.Ident().ProjectRoot
		newProject := true
		// Check if it's a new project, not in the old lock
		for _, y := range a.origL.Projects() {
			if pr == y.Ident().ProjectRoot {
				newProject = false
			}
		}
		if newProject {
			// Check if it's in notondisk project map. These are direct deps, should
			// be added to manifest.
			if _, ok := a.pd.notondisk[pr]; ok {
				m.Dependencies[pr] = getProjectPropertiesFromVersion(x.Version())
				feedback(x.Version(), pr, fb.DepTypeDirect, a.ctx)
			} else {
				// Log feedback of transitive project
				feedback(x.Version(), pr, fb.DepTypeTransitive, a.ctx)
			}
		}
	}

	// Remove dependencies from the manifest that aren't used
	for pr := range m.Dependencies {
		var used bool
		for _, y := range l.Projects() {
			if pr == y.Ident().ProjectRoot {
				used = true
				break
			}
		}
		if !used {
			delete(m.Dependencies, pr)
		}
	}
}
