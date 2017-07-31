// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

const godepJSONName = "Godeps.json"

type godepImporter struct {
	json godepJSON

	logger  *log.Logger
	verbose bool
	sm      gps.SourceManager
}

func newGodepImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *godepImporter {
	return &godepImporter{
		logger:  logger,
		verbose: verbose,
		sm:      sm,
	}
}

type godepJSON struct {
	Imports []godepPackage `json:"Deps"`
}

type godepPackage struct {
	ImportPath string `json:"ImportPath"`
	Rev        string `json:"Rev"`
	Comment    string `json:"Comment"`
}

func (g *godepImporter) Name() string {
	return "godep"
}

func (g *godepImporter) HasDepMetadata(dir string) bool {
	y := filepath.Join(dir, "Godeps", godepJSONName)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	return true
}

func (g *godepImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return g.convert(pr)
}

func (g *godepImporter) load(projectDir string) error {
	g.logger.Println("Detected godep configuration files...")
	j := filepath.Join(projectDir, "Godeps", godepJSONName)
	if g.verbose {
		g.logger.Printf("  Loading %s", j)
	}
	jb, err := ioutil.ReadFile(j)
	if err != nil {
		return errors.Wrapf(err, "Unable to read %s", j)
	}
	err = json.Unmarshal(jb, &g.json)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse %s", j)
	}

	return nil
}

func (g *godepImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.logger.Println("Converting from Godeps.json ...")

	manifest := &dep.Manifest{
		Constraints: make(gps.ProjectConstraints),
	}
	lock := &dep.Lock{}

	for _, pkg := range g.json.Imports {
		// ImportPath must not be empty
		if pkg.ImportPath == "" {
			err := errors.New("Invalid godep configuration, ImportPath is required")
			return nil, nil, err
		}

		// Obtain ProjectRoot. Required for avoiding sub-package imports.
		ip, err := g.sm.DeduceProjectRoot(pkg.ImportPath)
		if err != nil {
			return nil, nil, err
		}
		pkg.ImportPath = string(ip)

		// Check if it already existing in locked projects
		if projectExistsInLock(lock, pkg.ImportPath) {
			continue
		}

		// Rev must not be empty
		if pkg.Rev == "" {
			err := errors.New("Invalid godep configuration, Rev is required")
			return nil, nil, err
		}

		if pkg.Comment == "" {
			// When there's no comment, try to get corresponding version for the Rev
			// and fill Comment.
			pi := gps.ProjectIdentifier{
				ProjectRoot: gps.ProjectRoot(pkg.ImportPath),
			}
			revision := gps.Revision(pkg.Rev)

			version, err := lookupVersionForLockedProject(pi, nil, revision, g.sm)
			if err != nil {
				// Only warn about the problem, it is not enough to warrant failing
				g.logger.Println(err.Error())
			} else {
				pp := getProjectPropertiesFromVersion(version)
				if pp.Constraint != nil {
					pkg.Comment = pp.Constraint.String()
				}
			}
		}

		if pkg.Comment != "" {
			// If there's a comment, use it to create project constraint
			pc, err := g.buildProjectConstraint(pkg)
			if err != nil {
				return nil, nil, err
			}
			manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{Constraint: pc.Constraint}
		}

		lp := g.buildLockedProject(pkg, manifest)
		lock.P = append(lock.P, lp)
	}

	return manifest, lock, nil
}

// buildProjectConstraint uses the provided package ImportPath and Comment to
// create a project constraint
func (g *godepImporter) buildProjectConstraint(pkg godepPackage) (pc gps.ProjectConstraint, err error) {
	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.ImportPath)}
	pc.Constraint, err = g.sm.InferConstraint(pkg.Comment, pc.Ident)
	if err != nil {
		return
	}

	f := fb.NewConstraintFeedback(pc, fb.DepTypeImported)
	f.LogFeedback(g.logger)

	return
}

// buildLockedProject uses the package Rev and Comment to create lock project
func (g *godepImporter) buildLockedProject(pkg godepPackage, manifest *dep.Manifest) gps.LockedProject {
	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.ImportPath)}
	revision := gps.Revision(pkg.Rev)
	pp := manifest.Constraints[pi.ProjectRoot]

	version, err := lookupVersionForLockedProject(pi, pp.Constraint, revision, g.sm)
	if err != nil {
		// Only warn about the problem, it is not enough to warrant failing
		g.logger.Println(err.Error())
	}

	lp := gps.NewLockedProject(pi, version, nil)
	f := fb.NewLockedProjectFeedback(lp, fb.DepTypeImported)
	f.LogFeedback(g.logger)

	return lp
}

// projectExistsInLock checks if the given import path already existing in
// locked projects.
func projectExistsInLock(l *dep.Lock, ip string) bool {
	for _, lp := range l.P {
		if ip == string(lp.Ident().ProjectRoot) {
			return true
		}
	}

	return false
}
