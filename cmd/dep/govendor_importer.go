// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

const govendorDir = "vendor"
const govendorName = "vendor.json"

type govendorImporter struct {
	file govendorFile

	logger  *log.Logger
	verbose bool
	sm      gps.SourceManager
}

func newGovendorImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *govendorImporter {
	return &govendorImporter{
		logger:  logger,
		verbose: verbose,
		sm:      sm,
	}
}

// File is the structure of the vendor file.
type govendorFile struct {
	RootPath string // Import path of vendor folder
	Ignore   string
	Package  []*govendorPackage
}

// Package represents each package.
type govendorPackage struct {
	// See the vendor spec for definitions.
	Origin   string
	Path     string
	Revision string
	Version  string
}

func (g *govendorImporter) Name() string {
	return "govendor"
}

func (g *govendorImporter) HasDepMetadata(dir string) bool {
	y := filepath.Join(dir, govendorDir, govendorName)
	if _, err := os.Stat(y); err != nil {
		return false
	}
	return true
}

func (g *govendorImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}
	return g.convert(pr)
}

func (g *govendorImporter) load(projectDir string) error {
	g.logger.Println("Detected govendor configuration file...")
	v := filepath.Join(projectDir, govendorDir, govendorName)
	if g.verbose {
		g.logger.Printf("  Loading %s", v)
	}
	vb, err := ioutil.ReadFile(v)
	if err != nil {
		return errors.Wrapf(err, "Unable to read %s", v)
	}
	err = json.Unmarshal(vb, &g.file)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse %s", v)
	}
	return nil
}

func (g *govendorImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.logger.Println("Converting from vendor.json...")

	manifest := dep.NewManifest()

	if len(g.file.Ignore) > 0 {
		// Govendor has three use cases here
		// 1. 'test' - special case for ignoring test files
		// 2. build tags - any string without a slash (/) in it
		// 3. path and path prefix - any string with a slash (/) in it.
		//   The path case could be a full path or just a prefix.
		// Dep doesn't support build tags right now: https://github.com/golang/dep/issues/120
		for _, i := range strings.Split(g.file.Ignore, " ") {
			if !strings.Contains(i, "/") {
				g.logger.Printf("  Govendor was configured to ignore the %s build tag, but that isn't supported by dep yet, and will be ignored. See https://github.com/golang/dep/issues/291.", i)
				continue
			}
			_, err := g.sm.DeduceProjectRoot(i)
			if err == nil {
				manifest.Ignored = append(manifest.Ignored, i)
			} else {
				g.logger.Printf("  Govendor was configured to ignore the %s package prefix, but that isn't supported by dep yet, and will be ignored.", i)
			}
		}
	}

	lock := &dep.Lock{}
	for _, pkg := range g.file.Package {
		// Path must not be empty
		if pkg.Path == "" {
			err := errors.New("Invalid govendor configuration, Path is required")
			return nil, nil, err
		}

		// Obtain ProjectRoot. Required for avoiding sub-package imports.
		// Use Path instead of Origin since we are trying to group by project here
		pr, err := g.sm.DeduceProjectRoot(pkg.Path)
		if err != nil {
			return nil, nil, err
		}
		pkg.Path = string(pr)

		// Check if it already existing in locked projects
		if projectExistsInLock(lock, pr) {
			continue
		}

		// Revision must not be empty
		if pkg.Revision == "" {
			err := errors.New("Invalid govendor configuration, Revision is required")
			return nil, nil, err
		}

		if pkg.Version == "" {
			// When no version is specified try to get the corresponding version
			pi := gps.ProjectIdentifier{
				ProjectRoot: pr,
			}
			if pkg.Origin != "" {
				pi.Source = pkg.Origin
			}
			revision := gps.Revision(pkg.Revision)
			version, err := lookupVersionForLockedProject(pi, nil, revision, g.sm)
			if err != nil {
				// Only warn about the problem, it is not enough to warrant failing
				g.logger.Println(err.Error())
			} else {
				pp := getProjectPropertiesFromVersion(version)
				if pp.Constraint != nil {
					pkg.Version = pp.Constraint.String()
				}
			}
		}

		// If there's a version, use it to create project constraint
		pc, err := g.buildProjectConstraint(pkg)
		if err != nil {
			return nil, nil, err
		}
		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{Constraint: pc.Constraint}

		lp := g.buildLockedProject(pkg, manifest)
		lock.P = append(lock.P, lp)
	}

	return manifest, lock, nil
}

func (g *govendorImporter) buildProjectConstraint(pkg *govendorPackage) (pc gps.ProjectConstraint, err error) {
	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Path), Source: pkg.Path}

	if pkg.Version != "" {
		pc.Constraint, err = g.sm.InferConstraint(pkg.Version, pc.Ident)
		if err != nil {
			return
		}
	} else {
		pc.Constraint = gps.Any()
	}

	f := fb.NewConstraintFeedback(pc, fb.DepTypeImported)
	f.LogFeedback(g.logger)

	return
}

func (g *govendorImporter) buildLockedProject(pkg *govendorPackage, manifest *dep.Manifest) gps.LockedProject {
	pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Path)}
	revision := gps.Revision(pkg.Revision)
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
