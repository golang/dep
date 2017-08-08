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
	logger *log.Logger
	sm     gps.SourceManager
}

func newVndrImporter(log *log.Logger, verbose bool, sm gps.SourceManager) *vndrImporter {
	return &vndrImporter{
		logger: log,
		sm:     sm,
	}
}

func (v *vndrImporter) Name() string { return "vndr" }

func (v *vndrImporter) HasDepMetadata(dir string) bool {
	_, err := os.Stat(vndrFile(dir))
	return err == nil
}

func (v *vndrImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	v.logger.Println("Converting from vendor.conf")

	packages, err := parseVndrFile(dir)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to parse vndr file")
	}

	manifest := &dep.Manifest{
		Constraints: make(gps.ProjectConstraints),
	}
	lock := &dep.Lock{}

	for _, pkg := range packages {
		pc := gps.ProjectConstraint{
			Ident: gps.ProjectIdentifier{
				ProjectRoot: gps.ProjectRoot(pkg.importPath),
				Source:      pkg.repository,
			},
		}
		pc.Constraint, err = v.sm.InferConstraint(pkg.revision, pc.Ident)
		if err != nil {
			pc.Constraint = gps.Revision(pkg.revision)
		}

		fb.NewConstraintFeedback(pc, fb.DepTypeImported).LogFeedback(v.logger)

		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			Source:     pc.Ident.Source,
			Constraint: pc.Constraint,
		}

		revision := gps.Revision(pkg.revision)
		version, err := lookupVersionForLockedProject(pc.Ident, pc.Constraint, revision, v.sm)
		if err != nil {
			v.logger.Println(err.Error())
		}

		lp := gps.NewLockedProject(pc.Ident, version, nil)

		fb.NewLockedProjectFeedback(lp, fb.DepTypeImported).LogFeedback(v.logger)
		lock.P = append(lock.P)
	}

	return manifest, lock, nil
}

func parseVndrFile(dir string) ([]vndrPackage, error) {
	f, err := os.Open(vndrFile(dir))
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to open %s", vndrFile(dir))
	}
	defer f.Close()

	var packages []vndrPackage
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		pkg, err := parseVndrLine(scanner.Text())
		if err != nil {
			return nil, errors.Wrapf(err, "unable to parse line")
		}
		if pkg == nil {
			// Could be an empty line or one which is just a comment
			continue
		}
		packages = append(packages, *pkg)
	}

	if scanner.Err() != nil {
		return nil, errors.Wrapf(err, "unable to read %s", vndrFile(dir))
	}

	return packages, nil
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
		revision:   parts[1],
	}
	if len(parts) == 3 {
		pkg.repository = parts[2]
	}

	return pkg, nil
}

type vndrPackage struct {
	importPath string
	revision   string
	repository string
}
