// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"log"

	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/importers"
)

// rootAnalyzer supplies manifest/lock data from both dep and external tool's
// configuration files.
// * When used on the root project, it imports only from external tools.
// * When used by the solver for dependencies, it first looks for dep config,
//   then external tools.
type rootAnalyzer struct {
	skipTools  bool
	ctx        *dep.Ctx
	sm         gps.SourceManager
	directDeps map[string]bool
}

func newRootAnalyzer(skipTools bool, ctx *dep.Ctx, directDeps map[string]bool, sm gps.SourceManager) *rootAnalyzer {
	return &rootAnalyzer{
		skipTools:  skipTools,
		ctx:        ctx,
		sm:         sm,
		directDeps: directDeps,
	}
}

func (a *rootAnalyzer) InitializeRootManifestAndLock(dir string, pr gps.ProjectRoot) (rootM *dep.Manifest, rootL *dep.Lock, err error) {
	if !a.skipTools {
		rootM, rootL, err = a.importManifestAndLock(dir, pr, false)
		if err != nil {
			return
		}
	}

	if rootM == nil {
		rootM = dep.NewManifest()
	}
	if rootL == nil {
		rootL = &dep.Lock{}
	}

	return
}

func (a *rootAnalyzer) importManifestAndLock(dir string, pr gps.ProjectRoot, suppressLogs bool) (*dep.Manifest, *dep.Lock, error) {
	logger := a.ctx.Err
	if suppressLogs {
		logger = log.New(ioutil.Discard, "", 0)
	}

	for _, i := range importers.BuildAll(logger, a.ctx.Verbose, a.sm) {
		if i.HasDepMetadata(dir) {
			a.ctx.Err.Printf("Importing configuration from %s. These are only initial constraints, and are further refined during the solve process.", i.Name())
			m, l, err := i.Import(dir, pr)
			if err != nil {
				return nil, nil, err
			}
			a.removeTransitiveDependencies(m)
			return m, l, err
		}
	}

	var emptyManifest = dep.NewManifest()

	return emptyManifest, nil, nil
}

func (a *rootAnalyzer) removeTransitiveDependencies(m *dep.Manifest) {
	for pr := range m.Constraints {
		if _, isDirect := a.directDeps[string(pr)]; !isDirect {
			delete(m.Constraints, pr)
		}
	}
}

// DeriveManifestAndLock evaluates a dependency for existing dependency manager
// configuration (ours or external) and passes any configuration found back
// to the solver.
func (a *rootAnalyzer) DeriveManifestAndLock(dir string, pr gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	// Ignore other tools if we find dep configuration
	var depAnalyzer dep.Analyzer
	if depAnalyzer.HasDepMetadata(dir) {
		return depAnalyzer.DeriveManifestAndLock(dir, pr)
	}

	if !a.skipTools {
		// The assignment back to an interface prevents interface-based nil checks from failing later
		var manifest gps.Manifest = gps.SimpleManifest{}
		var lock gps.Lock
		im, il, err := a.importManifestAndLock(dir, pr, true)
		if im != nil {
			manifest = im
		}
		if il != nil {
			lock = il
		}
		return manifest, lock, err
	}

	return gps.SimpleManifest{}, nil, nil
}

func (a *rootAnalyzer) FinalizeRootManifestAndLock(m *dep.Manifest, l *dep.Lock, ol dep.Lock) {
	// Iterate through the new projects in solved lock and add them to manifest
	// if they are direct deps and log feedback for all the new projects.
	for _, y := range l.Projects() {
		var f *fb.ConstraintFeedback
		pr := y.Ident().ProjectRoot
		// New constraints: in new lock and dir dep but not in manifest
		if _, ok := a.directDeps[string(pr)]; ok {
			if _, ok := m.Constraints[pr]; !ok {
				pp := getProjectPropertiesFromVersion(y.Version())
				if pp.Constraint != nil {
					m.Constraints[pr] = pp
					pc := gps.ProjectConstraint{Ident: y.Ident(), Constraint: pp.Constraint}
					f = fb.NewConstraintFeedback(pc, fb.DepTypeDirect)
					f.LogFeedback(a.ctx.Err)
				}
				f = fb.NewLockedProjectFeedback(y, fb.DepTypeDirect)
				f.LogFeedback(a.ctx.Err)
			}
		} else {
			// New locked projects: in new lock but not in old lock
			newProject := true
			for _, opl := range ol.Projects() {
				if pr == opl.Ident().ProjectRoot {
					newProject = false
				}
			}
			if newProject {
				f = fb.NewLockedProjectFeedback(y, fb.DepTypeTransitive)
				f.LogFeedback(a.ctx.Err)
			}
		}
	}
}

// Info provides metadata on the analyzer algorithm used during solve.
func (a *rootAnalyzer) Info() gps.ProjectAnalyzerInfo {
	return gps.ProjectAnalyzerInfo{
		Name:    "dep",
		Version: 1,
	}
}
