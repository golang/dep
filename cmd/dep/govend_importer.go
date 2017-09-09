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
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

// ToDo: govend supports json and xml formats as well and we will add support for other formats in next PR - @RaviTezu
// govend don't have a separate lock file.
const govendYAMLName = "vendor.yml"

// govendImporter imports govend configuration in to the dep configuration format.
type govendImporter struct {
	*baseImporter
	yaml govendYAML
}

func newGovendImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *govendImporter {
	return &govendImporter{baseImporter: newBaseImporter(logger, verbose, sm)}
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
		g.logger.Printf("  Loading %s", y)
	}
	yb, err := ioutil.ReadFile(y)
	if err != nil {
		return errors.Wrapf(err, "unable to read %s", y)
	}
	err = yaml.Unmarshal(yb, &g.yaml)
	if err != nil {
		return errors.Wrapf(err, "unable to parse %s", y)
	}
	return nil
}

// convert the govend configuration files into dep configuration files.
func (g *govendImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.logger.Println("Converting from vendor.yaml...")

	packages := make([]importedPackage, 0, len(g.yaml.Imports))
	for _, pkg := range g.yaml.Imports {
		// Path must not be empty
		if pkg.Path == "" || pkg.Revision == "" {
			return nil, nil, errors.New("invalid govend configuration, path and rev are required")
		}

		ip := importedPackage{
			Name:     pkg.Path,
			LockHint: pkg.Revision,
		}
		packages = append(packages, ip)
	}

	err := g.importPackages(packages, true)
	if err != nil {
		return nil, nil, err
	}

	return g.manifest, g.lock, nil
}
