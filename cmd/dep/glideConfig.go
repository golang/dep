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
	lock    glideLock
	loggers *Loggers
}

func newGlideFiles(loggers *Loggers) glideFiles {
	return glideFiles{loggers: loggers}
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
}

func (files glideFiles) load(projectDir string) error {
	y := filepath.Join(projectDir, glideYamlName)
	if files.loggers.Verbose {
		files.loggers.Err.Printf("dep: Loading %s", y)
	}
	yb, err := ioutil.ReadFile(y)
	if err != nil {
		return errors.Wrapf(err, "Unable to read %s", y)
	}
	err = yaml.Unmarshal(yb, &files.yaml)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse %s", y)
	}

	l := filepath.Join(projectDir, glideLockName)
	if files.loggers.Verbose {
		files.loggers.Err.Printf("dep: Loading %s", l)
	}
	lb, err := ioutil.ReadFile(l)
	if err != nil {
		return errors.Wrapf(err, "Unable to read %s", l)
	}
	err = yaml.Unmarshal(lb, &files.lock)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse %s", l)
	}

	return nil
}

func (files glideFiles) convert(projectName string) (*dep.Manifest, *dep.Lock, error) {
	manifest := &dep.Manifest{
		Dependencies: make(gps.ProjectConstraints),
	}

	constrainDep := func(pkg glidePackage) {
		manifest.Dependencies[gps.ProjectRoot(pkg.Name)] = gps.ProjectProperties{
			Source:     pkg.Repository,
			Constraint: deduceConstraint(pkg.Reference),
		}
	}
	for _, pkg := range files.yaml.Imports {
		constrainDep(pkg)
	}
	for _, pkg := range files.yaml.TestImports {
		constrainDep(pkg)
	}

	manifest.Ignored = append(manifest.Ignored, files.yaml.Ignores...)

	if len(files.yaml.ExcludeDirs) > 0 {
		if files.yaml.Name != "" && files.yaml.Name != projectName {
			files.loggers.Err.Printf("dep: Glide thinks the package is '%s' but dep thinks it is '%s', using dep's value.\n", files.yaml.Name, projectName)
		}

		for _, dir := range files.yaml.ExcludeDirs {
			pkg := path.Join(projectName, dir)
			manifest.Ignored = append(manifest.Ignored, pkg)
		}
	}

	lock := &dep.Lock{}

	lockDep := func(pkg glidePackage) {
		id := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Name)}
		c, has := manifest.Dependencies[id.ProjectRoot]
		if has {
			id.Source = c.Source
		}
		version := gps.Revision(pkg.Reference)

		lp := gps.NewLockedProject(id, version, nil)
		lock.P = append(lock.P, lp)
	}
	for _, pkg := range files.lock.Imports {
		lockDep(pkg)
	}
	for _, pkg := range files.lock.TestImports {
		lockDep(pkg)
	}

	return manifest, lock, nil
}
