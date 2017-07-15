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
	Tree     bool
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

	manifest := &dep.Manifest{
		Constraints: make(gps.ProjectConstraints),
	}

	if len(g.file.Ignore) > 0 {
		manifest.Ignored = strings.Split(g.file.Ignore, " ")
	}

	for _, pkg := range g.file.Package {
		pc, err := g.buildProjectConstraint(pkg)
		if err != nil {
			return nil, nil, err
		}
		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			Source:     pc.Ident.Source,
			Constraint: pc.Constraint,
		}
	}
	return manifest, nil, nil
}

func (g *govendorImporter) buildProjectConstraint(pkg *govendorPackage) (pc gps.ProjectConstraint, err error) {
	if pkg.Path == "" {
		err = errors.New("Invalid vendor configuration, package path is required")
		return
	}

	ref := pkg.Version
	if ref == "" {
		ref = pkg.Revision
	}

	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Path), Source: pkg.Path}
	pc.Constraint, err = g.sm.InferConstraint(ref, pc.Ident)
	if err != nil {
		return
	}

	f := fb.NewConstraintFeedback(pc, fb.DepTypeImported)
	f.LogFeedback(g.logger)

	return
}
