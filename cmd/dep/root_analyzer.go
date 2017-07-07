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
	"github.com/pkg/errors"
)

// importer handles importing configuration from other dependency managers into
// the dep configuration format.
type importer interface {
	Name() string
	Import(path string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error)
	HasDepMetadata(dir string) bool
}

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
		rootM = &dep.Manifest{
			Constraints: make(gps.ProjectConstraints),
			Ovr:         make(gps.ProjectConstraints),
		}
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

	importers := []importer{
		newGlideImporter(logger, a.ctx.Verbose, a.sm),
		newGodepImporter(logger, a.ctx.Verbose, a.sm),
	}

	for _, i := range importers {
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

	var emptyManifest = &dep.Manifest{Constraints: make(gps.ProjectConstraints), Ovr: make(gps.ProjectConstraints)}
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

func (a *rootAnalyzer) Info() gps.ProjectAnalyzerInfo {
	name := "dep"
	version := 1
	if !a.skipTools {
		name = "dep+import"
	}
	return gps.ProjectAnalyzerInfo{
		Name:    name,
		Version: version,
	}
}

// lookupVersionForLockedProject figures out the appropriate version for a locked
// project based on the locked revision and the constraint from the manifest.
// First try matching the revision to a version, then try the constraint from the
// manifest, then finally the revision.
func lookupVersionForLockedProject(pi gps.ProjectIdentifier, c gps.Constraint, rev gps.Revision, sm gps.SourceManager) (gps.Version, error) {
	// Find the version that goes with this revision, if any
	versions, err := sm.ListVersions(pi)
	if err != nil {
		return rev, errors.Wrapf(err, "Unable to lookup the version represented by %s in %s(%s). Falling back to locking the revision only.", rev, pi.ProjectRoot, pi.Source)
	}

	gps.SortPairedForUpgrade(versions) // Sort versions in asc order
	for _, v := range versions {
		if v.Revision() == rev {
			// If the constraint is semver, make sure the version is acceptable.
			// This prevents us from suggesting an incompatible version, which
			// helps narrow the field when there are multiple matching versions.
			if c != nil {
				_, err := gps.NewSemverConstraint(c.String())
				if err == nil && !c.Matches(v) {
					continue
				}
			}
			return v, nil
		}
	}

	// Use the version from the manifest as long as it wasn't a range
	switch tv := c.(type) {
	case gps.PairedVersion:
		return tv.Unpair().Pair(rev), nil
	case gps.UnpairedVersion:
		return tv.Pair(rev), nil
	}

	// Give up and lock only to a revision
	return rev, nil
}
