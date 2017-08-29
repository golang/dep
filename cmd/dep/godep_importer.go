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
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

const godepPath = "Godeps" + string(os.PathSeparator) + "Godeps.json"

type godepImporter struct {
	*baseImporter
	json godepJSON
}

func newGodepImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *godepImporter {
	return &godepImporter{baseImporter: newBaseImporter(logger, verbose, sm)}
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
	y := filepath.Join(dir, godepPath)
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
	j := filepath.Join(projectDir, godepPath)
	if g.verbose {
		g.logger.Printf("  Loading %s", j)
	}
	jb, err := ioutil.ReadFile(j)
	if err != nil {
		return errors.Wrapf(err, "unable to read %s", j)
	}
	err = json.Unmarshal(jb, &g.json)
	if err != nil {
		return errors.Wrapf(err, "unable to parse %s", j)
	}

	return nil
}

func (g *godepImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.logger.Println("Converting from Godeps.json ...")

	packages := make([]importedPackage, 0, len(g.json.Imports))
	for _, pkg := range g.json.Imports {
		// Validate
		if pkg.ImportPath == "" {
			err := errors.New("invalid godep configuration, ImportPath is required")
			return nil, nil, err
		}

		if pkg.Rev == "" {
			err := errors.New("invalid godep configuration, Rev is required")
			return nil, nil, err
		}

		ip := importedPackage{
			Name:           pkg.ImportPath,
			LockHint:       pkg.Rev,
			ConstraintHint: pkg.Comment,
		}
		packages = append(packages, ip)
	}

	err := g.importPackages(packages, true)
	if err != nil {
		return nil, nil, err
	}

	return g.manifest, g.lock, nil
}
