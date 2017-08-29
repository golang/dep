// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/go-yaml/yaml"
	"github.com/golang/dep"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

const glideYamlName = "glide.yaml"
const glideLockName = "glide.lock"

// glideImporter imports glide configuration into the dep configuration format.
type glideImporter struct {
	*baseImporter
	glideConfig glideYaml
	glideLock   glideLock
	lockFound   bool
}

func newGlideImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *glideImporter {
	return &glideImporter{baseImporter: newBaseImporter(logger, verbose, sm)}
}

type glideYaml struct {
	Name        string         `yaml:"package"`
	Ignores     []string       `yaml:"ignore"`
	ExcludeDirs []string       `yaml:"excludeDirs"`
	Imports     []glidePackage `yaml:"import"`
	TestImports []glidePackage `yaml:"testImport"`
}

type glideLock struct {
	Imports     []glideLockedPackage `yaml:"imports"`
	TestImports []glideLockedPackage `yaml:"testImports"`
}

type glidePackage struct {
	Name       string `yaml:"package"`
	Reference  string `yaml:"version"` // could contain a semver, tag or branch
	Repository string `yaml:"repo"`

	// Unsupported fields that we will warn if used
	Subpackages []string `yaml:"subpackages"`
	OS          string   `yaml:"os"`
	Arch        string   `yaml:"arch"`
}

type glideLockedPackage struct {
	Name       string `yaml:"name"`
	Revision   string `yaml:"version"`
	Repository string `yaml:"repo"`
}

func (g *glideImporter) Name() string {
	return "glide"
}

func (g *glideImporter) HasDepMetadata(dir string) bool {
	// Only require glide.yaml, the lock is optional
	y := filepath.Join(dir, glideYamlName)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	return true
}

func (g *glideImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return g.convert(pr)
}

// load the glide configuration files.
func (g *glideImporter) load(projectDir string) error {
	g.logger.Println("Detected glide configuration files...")
	y := filepath.Join(projectDir, glideYamlName)
	if g.verbose {
		g.logger.Printf("  Loading %s", y)
	}
	yb, err := ioutil.ReadFile(y)
	if err != nil {
		return errors.Wrapf(err, "unable to read %s", y)
	}
	err = yaml.Unmarshal(yb, &g.glideConfig)
	if err != nil {
		return errors.Wrapf(err, "unable to parse %s", y)
	}

	l := filepath.Join(projectDir, glideLockName)
	if exists, _ := fs.IsRegular(l); exists {
		if g.verbose {
			g.logger.Printf("  Loading %s", l)
		}
		g.lockFound = true
		lb, err := ioutil.ReadFile(l)
		if err != nil {
			return errors.Wrapf(err, "unable to read %s", l)
		}
		lock := glideLock{}
		err = yaml.Unmarshal(lb, &lock)
		if err != nil {
			return errors.Wrapf(err, "unable to parse %s", l)
		}
		g.glideLock = lock
	}

	return nil
}

// convert the glide configuration files into dep configuration files.
func (g *glideImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	projectName := string(pr)

	task := bytes.NewBufferString("Converting from glide.yaml")
	if g.lockFound {
		task.WriteString(" and glide.lock")
	}
	task.WriteString("...")
	g.logger.Println(task)

	numPkgs := len(g.glideConfig.Imports) + len(g.glideConfig.TestImports) + len(g.glideLock.Imports) + len(g.glideLock.TestImports)
	packages := make([]importedPackage, 0, numPkgs)

	// Constraints
	for _, pkg := range append(g.glideConfig.Imports, g.glideConfig.TestImports...) {
		// Validate
		if pkg.Name == "" {
			return nil, nil, errors.New("invalid glide configuration: Name is required")
		}

		// Warn
		if g.verbose {
			if pkg.OS != "" {
				g.logger.Printf("  The %s package specified an os, but that isn't supported by dep yet, and will be ignored. See https://github.com/golang/dep/issues/291.\n", pkg.Name)
			}
			if pkg.Arch != "" {
				g.logger.Printf("  The %s package specified an arch, but that isn't supported by dep yet, and will be ignored. See https://github.com/golang/dep/issues/291.\n", pkg.Name)
			}
		}

		ip := importedPackage{
			Name:           pkg.Name,
			Source:         pkg.Repository,
			ConstraintHint: pkg.Reference,
		}
		packages = append(packages, ip)
	}

	// Locks
	for _, pkg := range append(g.glideLock.Imports, g.glideLock.TestImports...) {
		// Validate
		if pkg.Name == "" {
			return nil, nil, errors.New("invalid glide lock: Name is required")
		}

		ip := importedPackage{
			Name:     pkg.Name,
			Source:   pkg.Repository,
			LockHint: pkg.Revision,
		}
		packages = append(packages, ip)
	}

	err := g.importPackages(packages, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid glide configuration")
	}

	// Ignores
	g.manifest.Ignored = append(g.manifest.Ignored, g.glideConfig.Ignores...)
	if len(g.glideConfig.ExcludeDirs) > 0 {
		if g.glideConfig.Name != "" && g.glideConfig.Name != projectName {
			g.logger.Printf("  Glide thinks the package is '%s' but dep thinks it is '%s', using dep's value.\n", g.glideConfig.Name, projectName)
		}

		for _, dir := range g.glideConfig.ExcludeDirs {
			pkg := path.Join(projectName, dir)
			g.manifest.Ignored = append(g.manifest.Ignored, pkg)
		}
	}

	return g.manifest, g.lock, nil
}
