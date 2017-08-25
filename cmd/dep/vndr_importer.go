// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

func vndrFile(dir string) string {
	return filepath.Join(dir, "vendor.conf")
}

type vndrImporter struct {
	packages []vndrPackage

	logger  *log.Logger
	verbose bool
	sm      gps.SourceManager
}

func newVndrImporter(log *log.Logger, verbose bool, sm gps.SourceManager) *vndrImporter {
	return &vndrImporter{
		logger:  log,
		verbose: verbose,
		sm:      sm,
	}
}

func (v *vndrImporter) Name() string { return "vndr" }

func (v *vndrImporter) HasDepMetadata(dir string) bool {
	_, err := os.Stat(vndrFile(dir))
	return err == nil
}

func (v *vndrImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	v.logger.Println("Detected vndr configuration file...")

	err := v.loadVndrFile(dir)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to load vndr file")
	}

	return v.convert(pr)
}

func (v *vndrImporter) loadVndrFile(dir string) error {
	v.logger.Printf("Converting from vendor.conf...")

	f, err := os.Open(vndrFile(dir))
	if err != nil {
		return errors.Wrapf(err, "Unable to open %s", vndrFile(dir))
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		pkg, err := parseVndrLine(scanner.Text())
		if err != nil {
			return errors.Wrapf(err, "unable to parse line")
		}
		if pkg == nil {
			// Could be an empty line or one which is just a comment
			continue
		}
		v.packages = append(v.packages, *pkg)
	}

	if scanner.Err() != nil {
		return errors.Wrapf(err, "unable to read %s", vndrFile(dir))
	}

	return nil
}

func (v *vndrImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	var (
		manifest = dep.NewManifest()
		lock     = &dep.Lock{}
	)

	for _, pkg := range v.packages {
		if pkg.importPath == "" {
			err := errors.New("Invalid vndr configuration, import path is required")
			return nil, nil, err
		}

		// Obtain ProjectRoot. Required for avoiding sub-package imports.
		ip, err := v.sm.DeduceProjectRoot(pkg.importPath)
		if err != nil {
			return nil, nil, err
		}
		pkg.importPath = string(ip)

		// Check if it already existing in locked projects
		if projectExistsInLock(lock, ip) {
			continue
		}

		if pkg.reference == "" {
			err := errors.New("Invalid vndr configuration, revision is required")
			return nil, nil, err
		}

		pc := gps.ProjectConstraint{
			Ident: gps.ProjectIdentifier{
				ProjectRoot: gps.ProjectRoot(pkg.importPath),
				Source:      pkg.repository,
			},
			Constraint: gps.Any(),
		}

		// A vndr entry could contain either a version or a revision
		isVersion, version, err := isVersion(pc.Ident, pkg.reference, v.sm)
		if err != nil {
			return nil, nil, err
		}

		// If the reference is a revision, check if it is tagged with a version
		if !isVersion {
			revision := gps.Revision(pkg.reference)
			version, err = lookupVersionForLockedProject(pc.Ident, nil, revision, v.sm)
			if err != nil {
				v.logger.Println(err.Error())
			}
		}

		// Try to build a constraint from the version
		pp := getProjectPropertiesFromVersion(version)
		if pp.Constraint != nil {
			pc.Constraint = pp.Constraint
		}

		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			Source:     pc.Ident.Source,
			Constraint: pc.Constraint,
		}
		fb.NewConstraintFeedback(pc, fb.DepTypeImported).LogFeedback(v.logger)

		lp := gps.NewLockedProject(pc.Ident, version, nil)
		lock.P = append(lock.P, lp)
		fb.NewLockedProjectFeedback(lp, fb.DepTypeImported).LogFeedback(v.logger)
	}

	return manifest, lock, nil
}

type vndrPackage struct {
	importPath string
	reference  string
	repository string
}

func parseVndrLine(line string) (*vndrPackage, error) {
	commentIdx := strings.Index(line, "#")
	if commentIdx >= 0 {
		line = line[:commentIdx]
	}
	line = strings.TrimSpace(line)

	if line == "" {
		return nil, nil
	}

	parts := strings.Fields(line)

	if !(len(parts) == 2 || len(parts) == 3) {
		return nil, errors.Errorf("invalid config format: %q", line)
	}

	pkg := &vndrPackage{
		importPath: parts[0],
		reference:  parts[1],
	}
	if len(parts) == 3 {
		pkg.repository = parts[2]
	}

	return pkg, nil
}
