// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vndr

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/importers/base"
	"github.com/pkg/errors"
)

func vndrFile(dir string) string {
	return filepath.Join(dir, "vendor.conf")
}

// Importer imports vndr configuration into the dep configuration format.
type Importer struct {
	*base.Importer
	packages []vndrPackage
}

// NewImporter for vndr.
func NewImporter(log *log.Logger, verbose bool, sm gps.SourceManager) *Importer {
	return &Importer{Importer: base.NewImporter(log, verbose, sm)}
}

// Name of the importer.
func (v *Importer) Name() string { return "vndr" }

// HasDepMetadata checks if a directory contains config that the importer can handle.
func (v *Importer) HasDepMetadata(dir string) bool {
	_, err := os.Stat(vndrFile(dir))
	return err == nil
}

// Import the config found in the directory.
func (v *Importer) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	v.Logger.Println("Detected vndr configuration file...")

	err := v.loadVndrFile(dir)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "unable to load vndr file")
	}

	return v.convert(pr)
}

func (v *Importer) loadVndrFile(dir string) error {
	v.Logger.Printf("Converting from vendor.conf...")

	f, err := os.Open(vndrFile(dir))
	if err != nil {
		return errors.Wrapf(err, "unable to open %s", vndrFile(dir))
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

func (v *Importer) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	packages := make([]base.ImportedPackage, 0, len(v.packages))
	for _, pkg := range v.packages {
		// Validate
		if pkg.importPath == "" {
			err := errors.New("invalid vndr configuration: import path is required")
			return nil, nil, err
		}

		if pkg.reference == "" {
			err := errors.New("invalid vndr configuration: revision is required")
			return nil, nil, err
		}

		ip := base.ImportedPackage{
			Name:     pkg.importPath,
			Source:   pkg.repository,
			LockHint: pkg.reference,
		}
		packages = append(packages, ip)
	}
	err := v.ImportPackages(packages, true)
	if err != nil {
		return nil, nil, err
	}

	return v.Manifest, v.Lock, nil
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
