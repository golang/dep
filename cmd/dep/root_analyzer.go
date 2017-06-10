// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"

	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
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

func (a *rootAnalyzer) FinalizeRootManifestAndLock(m *dep.Manifest, l *dep.Lock) {
	// Remove dependencies from the manifest that aren't used
	for pr := range m.Constraints {
		var used bool
		for _, y := range l.Projects() {
			if pr == y.Ident().ProjectRoot {
				used = true
				break
			}
		}
		if !used {
			delete(m.Constraints, pr)
		}
	}
}

func (a *rootAnalyzer) Info() (string, int) {
	name := "dep"
	version := 1
	if !a.skipTools {
		name = "dep+import"
	}
	return name, version
}

// feedback logs project constraint as feedback to the user.
func feedback(v gps.Version, pr gps.ProjectRoot, depType string, logger *log.Logger) {
	rev, version, branch := gps.VersionComponentStrings(v)

	// Check if it's a valid SHA1 digest and trim to 7 characters.
	if len(rev) == 40 {
		if _, err := hex.DecodeString(rev); err == nil {
			// Valid SHA1 digest
			rev = rev[0:7]
		}
	}

	// Get LockedVersion
	var ver string
	if version != "" {
		ver = version
	} else if branch != "" {
		ver = branch
	}

	cf := &fb.ConstraintFeedback{
		LockedVersion:  ver,
		Revision:       rev,
		ProjectPath:    string(pr),
		DependencyType: depType,
	}

	// Get non-revision constraint if available
	if c := getProjectPropertiesFromVersion(v).Constraint; c != nil {
		cf.Version = c.String()
	}

	// Attach ConstraintType for direct/imported deps based on locked version
	if cf.DependencyType == fb.DepTypeDirect || cf.DependencyType == fb.DepTypeImported {
		if cf.LockedVersion != "" {
			cf.ConstraintType = fb.ConsTypeConstraint
		} else {
			cf.ConstraintType = fb.ConsTypeHint
		}
	}

	cf.LogFeedback(logger)
}

func lookupVersionForRevision(rev gps.Revision, pi gps.ProjectIdentifier, sm gps.SourceManager) (gps.Version, error) {
	// Find the version that goes with this revision, if any
	versions, err := sm.ListVersions(pi)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to list versions for %s(%s)", pi.ProjectRoot, pi.Source)
	}

	gps.SortPairedForUpgrade(versions) // Sort versions in asc order
	for _, v := range versions {
		if v.Underlying() == rev {
			return v, nil
		}
	}

	return rev, nil
}
