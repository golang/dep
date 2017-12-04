// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package glock

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/importers/base"
	"github.com/pkg/errors"
)

const glockfile = "GLOCKFILE"

// Importer imports glock configuration into the dep configuration format.
type Importer struct {
	*base.Importer

	packages []glockPackage
}

// NewImporter for glock.
func NewImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *Importer {
	return &Importer{Importer: base.NewImporter(logger, verbose, sm)}
}

// Name of the importer.
func (g *Importer) Name() string {
	return "glock"
}

// HasDepMetadata checks if a directory contains config that the importer can handle.
func (g *Importer) HasDepMetadata(dir string) bool {
	path := filepath.Join(dir, glockfile)
	if _, err := os.Stat(path); err != nil {
		return false
	}

	return true
}

// Import the config found in the directory.
func (g *Importer) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return g.convert(pr)
}

type glockPackage struct {
	importPath string
	revision   string
}

func (g *Importer) load(projectDir string) error {
	g.Logger.Println("Detected glock configuration files...")
	path := filepath.Join(projectDir, glockfile)
	if g.Verbose {
		g.Logger.Printf("  Loading %s", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "unable to open %s", path)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		pkg, err := parseGlockLine(scanner.Text())
		if err != nil {
			return err
		}
		if pkg == nil {
			continue
		}
		g.packages = append(g.packages, *pkg)
	}

	return nil
}

func parseGlockLine(line string) (*glockPackage, error) {
	fields := strings.Fields(line)
	switch len(fields) {
	case 2: // Valid.
	case 0: // Skip empty lines.
		return nil, nil
	default:
		return nil, fmt.Errorf("invalid glock configuration: %s", line)
	}

	// Skip commands.
	if fields[0] == "cmd" {
		return nil, nil
	}
	return &glockPackage{
		importPath: fields[0],
		revision:   fields[1],
	}, nil
}

func (g *Importer) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.Logger.Println("Converting from GLOCKFILE ...")

	packages := make([]base.ImportedPackage, 0, len(g.packages))
	for _, pkg := range g.packages {
		// Validate
		if pkg.importPath == "" {
			return nil, nil, errors.New("invalid glock configuration, import path is required")
		}

		if pkg.revision == "" {
			return nil, nil, errors.New("invalid glock configuration, revision is required")
		}

		packages = append(packages, base.ImportedPackage{
			Name:     pkg.importPath,
			LockHint: pkg.revision,
		})
	}

	err := g.ImportPackages(packages, true)
	if err != nil {
		return nil, nil, err
	}

	return g.Manifest, g.Lock, nil
}
