// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gvt

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/importers/base"
	"github.com/pkg/errors"
)

const gvtPath = "vendor" + string(os.PathSeparator) + "manifest"

// Importer imports gvt configuration into the dep configuration format.
type Importer struct {
	*base.Importer
	gvtConfig gvtManifest
}

// NewImporter for gvt.
func NewImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *Importer {
	return &Importer{Importer: base.NewImporter(logger, verbose, sm)}
}

type gvtManifest struct {
	Deps []gvtPkg `json:"dependencies"`
}

type gvtPkg struct {
	ImportPath string
	Repository string
	Revision   string
	Branch     string
}

// Name of the importer.
func (g *Importer) Name() string {
	return "gvt"
}

// HasDepMetadata checks if a directory contains config that the importer can handle.
func (g *Importer) HasDepMetadata(dir string) bool {
	y := filepath.Join(dir, gvtPath)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	return true
}

// Import the config found in the directory.
func (g *Importer) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return g.convert(pr)
}

func (g *Importer) load(projectDir string) error {
	g.Logger.Println("Detected gvt configuration files...")
	j := filepath.Join(projectDir, gvtPath)
	if g.Verbose {
		g.Logger.Printf("  Loading %s", j)
	}
	jb, err := ioutil.ReadFile(j)
	if err != nil {
		return errors.Wrapf(err, "unable to read %s", j)
	}
	err = json.Unmarshal(jb, &g.gvtConfig)
	if err != nil {
		return errors.Wrapf(err, "unable to parse %s", j)
	}

	return nil
}

func (g *Importer) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.Logger.Println("Converting from vendor/manifest ...")

	packages := make([]base.ImportedPackage, 0, len(g.gvtConfig.Deps))
	for _, pkg := range g.gvtConfig.Deps {
		// Validate
		if pkg.ImportPath == "" {
			err := errors.New("invalid gvt configuration, ImportPath is required")
			return nil, nil, err
		}

		if pkg.Revision == "" {
			err := errors.New("invalid gvt configuration, Revision is required")
			return nil, nil, err
		}

		var contstraintHint = ""
		if pkg.Branch != "master" {
			contstraintHint = pkg.Branch
		}

		ip := base.ImportedPackage{
			Name: pkg.ImportPath,
			//TODO: temporarly ignore .Repository. see https://github.com/golang/dep/pull/1166
			// Source:         pkg.Repository,
			LockHint:       pkg.Revision,
			ConstraintHint: contstraintHint,
		}
		packages = append(packages, ip)
	}

	err := g.ImportPackages(packages, true)
	if err != nil {
		return nil, nil, err
	}

	return g.Manifest, g.Lock, nil
}
