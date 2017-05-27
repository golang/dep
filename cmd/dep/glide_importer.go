// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/go-yaml/yaml"
	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

const glideYamlName = "glide.yaml"
const glideLockName = "glide.lock"

// glideImporter imports glide configuration into the dep configuration format.
type glideImporter struct {
	yaml glideYaml
	lock *glideLock

	ctx *dep.Ctx
	sm  gps.SourceManager
}

func newGlideImporter(ctx *dep.Ctx, sm gps.SourceManager) *glideImporter {
	return &glideImporter{
		ctx: ctx,
		sm:  sm,
	}
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
	Reference  string `yaml:"version"`
	Repository string `yaml:"repo"`

	// Unsupported fields that we will warn if used
	Subpackages []string `yaml:"subpackages"`
	OS          string   `yaml:"os"`
	Arch        string   `yaml:"arch"`
}

type glideLockedPackage struct {
	Name       string `yaml:"name"`
	Reference  string `yaml:"version"`
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
	g.ctx.Err.Println("Detected glide configuration files...")
	y := filepath.Join(projectDir, glideYamlName)
	if g.ctx.Loggers.Verbose {
		g.ctx.Loggers.Err.Printf("  Loading %s", y)
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
	if exists, _ := fs.IsRegular(l); exists {
		if g.ctx.Loggers.Verbose {
			g.ctx.Loggers.Err.Printf("  Loading %s", l)
		}
		lb, err := ioutil.ReadFile(l)
		if err != nil {
			return errors.Wrapf(err, "Unable to read %s", l)
		}
		lock := &glideLock{}
		err = yaml.Unmarshal(lb, lock)
		if err != nil {
			return errors.Wrapf(err, "Unable to parse %s", l)
		}
		g.lock = lock
	}

	return nil
}

// convert the glide configuration files into dep configuration files.
func (g *glideImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	projectName := string(pr)

	task := bytes.NewBufferString("Converting from glide.yaml")
	if g.lock != nil {
		task.WriteString(" and glide.lock")
	}
	task.WriteString("...")
	g.ctx.Loggers.Err.Println(task)

	manifest := &dep.Manifest{
		Constraints: make(gps.ProjectConstraints),
	}

	for _, pkg := range g.yaml.Imports {
		pc, err := g.buildProjectConstraint(pkg)
		if err != nil {
			return nil, nil, err
		}
		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{Source: pc.Ident.Source, Constraint: pc.Constraint}
	}
	for _, pkg := range g.yaml.TestImports {
		pc, err := g.buildProjectConstraint(pkg)
		if err != nil {
			return nil, nil, err
		}
		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{Source: pc.Ident.Source, Constraint: pc.Constraint}
	}

	manifest.Ignored = append(manifest.Ignored, g.yaml.Ignores...)

	if len(g.yaml.ExcludeDirs) > 0 {
		if g.yaml.Name != "" && g.yaml.Name != projectName {
			g.ctx.Loggers.Err.Printf("  Glide thinks the package is '%s' but dep thinks it is '%s', using dep's value.\n", g.yaml.Name, projectName)
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
			lp := g.buildLockedProject(pkg)
			lock.P = append(lock.P, lp)
		}
		for _, pkg := range g.lock.TestImports {
			lp := g.buildLockedProject(pkg)
			lock.P = append(lock.P, lp)
		}
	}

	return manifest, lock, nil
}

func (g *glideImporter) buildProjectConstraint(pkg glidePackage) (pc gps.ProjectConstraint, err error) {
	if pkg.Name == "" {
		err = errors.New("Invalid glide configuration, package name is required")
		return
	}

	if g.ctx.Loggers.Verbose {
		if pkg.OS != "" {
			g.ctx.Loggers.Err.Printf("  The %s package specified an os, but that isn't supported by dep yet, and will be ignored. See https://github.com/golang/dep/issues/291.\n", pkg.Name)
		}
		if pkg.Arch != "" {
			g.ctx.Loggers.Err.Printf("  The %s package specified an arch, but that isn't supported by dep yet, and will be ignored. See https://github.com/golang/dep/issues/291.\n", pkg.Name)
		}
	}

	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Name), Source: pkg.Repository}
	pc.Constraint, err = deduceConstraint(pkg.Reference, pc.Ident, g.sm)

	return
}

func (g *glideImporter) buildLockedProject(pkg glideLockedPackage) gps.LockedProject {
	pi := gps.ProjectIdentifier{
		ProjectRoot: gps.ProjectRoot(pkg.Name),
		Source:      pkg.Repository,
	}
	revision := gps.Revision(pkg.Reference)

	version, err := lookupVersionForRevision(revision, pi, g.sm)
	if err != nil {
		// Warn about the problem, it is not enough to warrant failing
		warn := errors.Wrapf(err, "Unable to lookup the version represented by %s in %s(%s). Falling back to locking the revision only.", revision, pi.ProjectRoot, pi.Source)
		g.ctx.Err.Printf(warn.Error())
		version = revision
	}

	feedback(version, pi.ProjectRoot, fb.DepTypeImported, g.ctx)
	return gps.NewLockedProject(pi, version, nil)
}
