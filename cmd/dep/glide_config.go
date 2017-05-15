// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"path"
	"path/filepath"

	"github.com/go-yaml/yaml"
	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

type glideFiles struct {
	yaml    glideYaml
	lock    *glideLock
	loggers *dep.Loggers
}

func newGlideFiles(loggers *dep.Loggers) *glideFiles {
	return &glideFiles{loggers: loggers}
}

type glideYaml struct {
	Name        string         `yaml:"package"`
	Ignores     []string       `yaml:"ignore"`
	ExcludeDirs []string       `yaml:"excludeDirs"`
	Imports     []glidePackage `yaml:"import"`
	TestImports []glidePackage `yaml:"testImport"`
}

type glideLock struct {
	Imports     []glidePackage `yaml:"import"`
	TestImports []glidePackage `yaml:"testImport"`
}

type glidePackage struct {
	Name       string `yaml:"package"`
	Reference  string `yaml:"version"`
	Repository string `yaml:"repo"`

	// Unsupported fields that we will warn if used
	Subpackages []string `yaml:"subpackages"`
	OS          string   `yaml:"os"`
	Arch        string   `yaml:"arch"`
}

func (g *glideFiles) load(projectDir string) error {
	y := filepath.Join(projectDir, glideYamlName)
	if g.loggers.Verbose {
		g.loggers.Err.Printf("glide: Loading %s", y)
	}
	yb, err := ioutil.ReadFile(y)
	if err != nil {
		return errors.Wrapf(err, "Unable to read %s", y)
	}
	err = yaml.Unmarshal(yb, &g.yaml)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse %s", y)
	}

	l := filepath.Join(projectDir, glideLockName)
	if exists, _ := dep.IsRegular(l); exists {
		if g.loggers.Verbose {
			g.loggers.Err.Printf("glide: Loading %s", l)
		}
		lb, err := ioutil.ReadFile(l)
		if err != nil {
			return errors.Wrapf(err, "Unable to read %s", l)
		}
		err = yaml.Unmarshal(lb, &g.lock)
		if err != nil {
			return errors.Wrapf(err, "Unable to parse %s", l)
		}
	}

	return nil
}

func (g *glideFiles) convert(projectName string, sm gps.SourceManager) (*dep.Manifest, *dep.Lock, error) {
	manifest := &dep.Manifest{
		Dependencies: make(gps.ProjectConstraints),
	}

	for _, pkg := range g.yaml.Imports {
		pc, err := g.buildProjectConstraint(pkg, sm)
		if err != nil {
			return nil, nil, err
		}
		manifest.Dependencies[pc.Ident.ProjectRoot] = gps.ProjectProperties{Source: pc.Ident.Source, Constraint: pc.Constraint}
	}
	for _, pkg := range g.yaml.TestImports {
		pc, err := g.buildProjectConstraint(pkg, sm)
		if err != nil {
			return nil, nil, err
		}
		manifest.Dependencies[pc.Ident.ProjectRoot] = gps.ProjectProperties{Source: pc.Ident.Source, Constraint: pc.Constraint}
	}

	manifest.Ignored = append(manifest.Ignored, g.yaml.Ignores...)

	if len(g.yaml.ExcludeDirs) > 0 {
		if g.yaml.Name != "" && g.yaml.Name != projectName {
			g.loggers.Err.Printf("dep: Glide thinks the package is '%s' but dep thinks it is '%s', using dep's value.\n", g.yaml.Name, projectName)
		}

		for _, dir := range g.yaml.ExcludeDirs {
			pkg := path.Join(projectName, dir)
			manifest.Ignored = append(manifest.Ignored, pkg)
		}
	}

	var lock *dep.Lock
	if g.lock != nil {
		lock = &dep.Lock{}

		for _, pkg := range g.lock.Imports {
			lp := g.buildLockedProject(pkg, manifest)
			lock.P = append(lock.P, lp)
		}
		for _, pkg := range g.lock.TestImports {
			lp := g.buildLockedProject(pkg, manifest)
			lock.P = append(lock.P, lp)
		}
	}

	return manifest, lock, nil
}

func (g *glideFiles) buildProjectConstraint(pkg glidePackage, sm gps.SourceManager) (pc gps.ProjectConstraint, err error) {
	if pkg.Name == "" {
		err = errors.New("glide: Invalid glide configuration, package name is required")
		return
	}

	if g.loggers.Verbose {
		if pkg.OS != "" {
			g.loggers.Err.Printf("glide: The %s package specified an os, but that isn't supported by dep, and will be ignored.\n", pkg.Name)
		}
		if pkg.Arch != "" {
			g.loggers.Err.Printf("glide: The %s package specified an arch, but that isn't supported by dep, and will be ignored.\n", pkg.Name)
		}
		if len(pkg.Subpackages) > 0 {
			g.loggers.Err.Printf("glide: The %s package specified subpackages, but that is calcuated by dep, and will be ignored.\n", pkg.Name)
		}
	}

	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Name), Source: pkg.Repository}
	pc.Constraint, err = deduceConstraint(pkg.Reference, pc.Ident, sm)
	return
}

func (g *glideFiles) buildLockedProject(pkg glidePackage, manifest *dep.Manifest) gps.LockedProject {
	id := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Name)}
	c, has := manifest.Dependencies[id.ProjectRoot]
	if has {
		id.Source = c.Source
	}
	version := gps.Revision(pkg.Reference)

	return gps.NewLockedProject(id, version, nil)
}
