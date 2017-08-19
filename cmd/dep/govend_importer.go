// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/go-yaml/yaml"
	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

// ToDo: govend supports json and xml formats as well and we will add support for other formats in next PR - @RaviTezu
// govend don't have a separate lock file.
const govendYAMLName = "vendor.yml"

// govendImporter imports govend configuration in to the dep configuration format.
type govendImporter struct {
	yaml govendYAML

	logger  *log.Logger
	verbose bool
	sm      gps.SourceManager
}

func newGovendImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *govendImporter {
	return &govendImporter{
		logger:  logger,
		verbose: verbose,
		sm:      sm,
	}
}

type govendYAML struct {
	Imports []govendPackage `yaml:"vendors"`
}

type govendPackage struct {
	Path     string `yaml:"path"`
	Revision string `yaml:"rev"`
}

func (g *govendImporter) Name() string {
	return "govend"
}

func (g *govendImporter) HasDepMetadata(dir string) bool {
	y := filepath.Join(dir, govendYAMLName)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	return true
}

func (g *govendImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return g.convert(pr)
}

// load the govend configuration files.
func (g *govendImporter) load(projectDir string) error {
	g.logger.Println("Detected govend configuration files...")
	y := filepath.Join(projectDir, govendYAMLName)
	if g.verbose {
		g.logger.Printf("	Loading %s", y)
	}
	yb, err := ioutil.ReadFile(y)
	if err != nil {
		return errors.Wrapf(err, "Unable to read %s", y)
	}
	err = yaml.Unmarshal(yb, &g.yaml)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse %s", y)
	}
	return nil
}

// convert the govend configuration files into dep configuration files.
func (g *govendImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.logger.Println("Converting from vendor.yaml...")

	manifest := &dep.Manifest{
		Constraints: make(gps.ProjectConstraints),
	}
	lock := &dep.Lock{}

	for _, pkg := range g.yaml.Imports {
		// Path must not be empty
		if pkg.Path == "" || pkg.Revision == "" {
			return nil, nil, errors.New("Invalid govend configuration, Path or Rev is required")
		}

		p, err := g.sm.DeduceProjectRoot(pkg.Path)
		if err != nil {
			return nil, nil, err
		}
		pkg.Path = string(p)

		// Check if the current project is already existing in locked projects.
		if projectExistsInLock(lock, p) {
			continue
		}

		pi := gps.ProjectIdentifier{
			ProjectRoot: gps.ProjectRoot(pkg.Path),
		}
		revision := gps.Revision(pkg.Revision)

		version, err := lookupVersionForLockedProject(pi, nil, revision, g.sm)
		if err != nil {
			g.logger.Println(err.Error())
		} else {
			pp := getProjectPropertiesFromVersion(version)
			if pp.Constraint != nil {
				pc, err := g.buildProjectConstraint(pkg, pp.Constraint.String())
				if err != nil {
					return nil, nil, err
				}
				manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{Constraint: pc.Constraint}
			}
		}

		lp := g.buildLockedProject(pkg, manifest)
		lock.P = append(lock.P, lp)
	}

	return manifest, lock, nil
}

func (g *govendImporter) buildProjectConstraint(pkg govendPackage, constraint string) (pc gps.ProjectConstraint, err error) {
	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Path)}
	pc.Constraint, err = g.sm.InferConstraint(constraint, pc.Ident)
	if err != nil {
		return
	}

	f := fb.NewConstraintFeedback(pc, fb.DepTypeImported)
	f.LogFeedback(g.logger)

	return

}

func (g *govendImporter) buildLockedProject(pkg govendPackage, manifest *dep.Manifest) gps.LockedProject {
	p := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Path)}
	revision := gps.Revision(pkg.Revision)
	pp := manifest.Constraints[p.ProjectRoot]

	version, err := lookupVersionForLockedProject(p, pp.Constraint, revision, g.sm)
	if err != nil {
		g.logger.Println(err.Error())
	}

	lp := gps.NewLockedProject(p, version, nil)
	f := fb.NewLockedProjectFeedback(lp, fb.DepTypeImported)
	f.LogFeedback(g.logger)

	return lp
}
