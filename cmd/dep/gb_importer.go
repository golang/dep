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
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

// gbImporter imports gb configuration into the dep configuration format.
type gbImporter struct {
	manifest gbManifest
	logger   *log.Logger
	verbose  bool
	sm       gps.SourceManager
}

func newGbImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *gbImporter {
	return &gbImporter{
		logger:  logger,
		verbose: verbose,
		sm:      sm,
	}
}

// gbManifest represents the manifest file for GB projects
type gbManifest struct {
	Dependencies []gbDependency `json:"dependencies"`
}

type gbDependency struct {
	Importpath string `json:"importpath"`
	Repository string `json:"repository"`

	// All gb vendored dependencies have a specific revision
	Revision string `json:"revision"`

	// Branch may be HEAD or an actual branch. In the case of HEAD, that means
	// the user vendored a dependency by specifying a tag or a specific revision
	// which results in a detached HEAD
	Branch string `json:"branch"`
}

func (i *gbImporter) Name() string {
	return "gb"
}

func (i *gbImporter) HasDepMetadata(dir string) bool {
	// gb stores the manifest in the vendor tree
	var m = filepath.Join(dir, "vendor", "manifest")
	if _, err := os.Stat(m); err != nil {
		return false
	}

	return true
}

func (i *gbImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := i.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return i.convert(pr)
}

// load the gb manifest
func (i *gbImporter) load(projectDir string) error {
	i.logger.Println("Detected gb manifest file...")
	var mf = filepath.Join(projectDir, "vendor", "manifest")
	if i.verbose {
		i.logger.Printf("  Loading %s", mf)
	}

	var buf []byte
	var err error
	if buf, err = ioutil.ReadFile(mf); err != nil {
		return errors.Wrapf(err, "Unable to read %s", mf)
	}
	if err := json.Unmarshal(buf, &i.manifest); err != nil {
		return errors.Wrapf(err, "Unable to parse %s", mf)
	}

	return nil
}

// convert the gb manifest into dep configuration files.
func (i *gbImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	i.logger.Println("Converting from gb manifest...")

	manifest := &dep.Manifest{
		Constraints: make(gps.ProjectConstraints),
	}

	lock := &dep.Lock{}

	for _, pkg := range i.manifest.Dependencies {
		if pkg.Importpath == "" {
			return nil, nil, errors.New("Invalid gb configuration, package import path is required")
		}

		if pkg.Revision == "" {
			return nil, nil, errors.New("Invalid gb configuration, package revision is required")
		}

		// Deduce the project root. This is necessary because gb manifests can have
		// multiple entries for the same project root, one for each imported subpackage
		var root gps.ProjectRoot
		var err error
		if root, err = i.sm.DeduceProjectRoot(pkg.Importpath); err != nil {
			return nil, nil, err
		}

		// Set the proper import path back on the dependency
		pkg.Importpath = string(root)

		// If we've already locked this project root then we can skip
		if projectExistsInLock(lock, pkg.Importpath) {
			continue
		}

		// Otherwise, attempt to convert this specific package, which returns a constraint and a lock
		pc, lp, err := i.convertOne(pkg)
		if err != nil {
			return nil, nil, err
		}

		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{Source: pc.Ident.Source, Constraint: pc.Constraint}
		lock.P = append(lock.P, lp)

	}

	return manifest, lock, nil
}

func (i *gbImporter) convertOne(pkg gbDependency) (pc gps.ProjectConstraint, lp gps.LockedProject, err error) {
	/*
		gb's vendor plugin (gb vendor), which manages the vendor tree and manifest
		file, supports fetching by a specific tag or revision, but if you specify
		either of those it's a detached checkout and the "branch" field is HEAD.
		The only time the "branch" field is not "HEAD" is if you do not specify a
		tag or revision, otherwise it's either "master" or the value of the -branch
		flag

		This means that, generally, the only possible "constraint" we can really specify is
		the branch name if the branch name is not HEAD. Otherwise, it's a specific revision.

		However, if we can infer a tag that points to the revision or the branch, we may be able
		to use that as the constraint

		So, the order of operations to convert a single dependency in a gb manifest is:
			- Find a specific version for the revision (and branch, if set)
			- If there's a branch available, use that as the constraint
			- If there's no branch, but we found a version from step 1, use the version as the constraint
	*/
	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.Importpath), Source: pkg.Repository}

	// Generally, gb tracks revisions
	var revision = gps.Revision(pkg.Revision)

	// But if the branch field is not "HEAD", we can use that as the initial constraint
	var constraint gps.Constraint
	if pkg.Branch != "" && pkg.Branch != "HEAD" {
		constraint = gps.NewBranch(pkg.Branch)
	}

	// See if we can get a version from that constraint
	version, err := lookupVersionForLockedProject(pc.Ident, constraint, revision, i.sm)
	if err != nil {
		// Log the error, but don't fail it. It's okay if we can't find a version
		i.logger.Println(err.Error())
	}

	// If the constraint is nil (no branch), but there's a version, infer a constraint from there
	if constraint == nil && version != nil {
		constraint, err = i.sm.InferConstraint(version.String(), pc.Ident)
		if err != nil {
			return
		}
	}

	// If there's *still* no constraint, set the constraint to the revision
	if constraint == nil {
		constraint = revision
	}

	pc.Constraint = constraint

	lp = gps.NewLockedProject(pc.Ident, version, nil)

	fb.NewConstraintFeedback(pc, fb.DepTypeImported).LogFeedback(i.logger)
	fb.NewLockedProjectFeedback(lp, fb.DepTypeImported).LogFeedback(i.logger)

	return
}
