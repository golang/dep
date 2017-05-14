// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
)

// importer
type importer interface {
	gps.ProjectAnalyzer
	rootProjectAnalyzer
	HasConfig(dir string) bool
}

// importAnalyzer imports existing dependency management configuration,
// from both dep and external tools.
type importAnalyzer struct {
	loggers *dep.Loggers
	sm      gps.SourceManager
}

func newImportAnalyzer(loggers *dep.Loggers, sm gps.SourceManager) importAnalyzer {
	return importAnalyzer{loggers: loggers, sm: sm}
}

func (a importAnalyzer) Info() (string, int) {
	// TODO: do not merge until this is set to something unique.
	// I'm not changing it now because that will cause the memo to change in tests
	// which I'll deal with and update later
	return "dep", 1
}

func (a importAnalyzer) DeriveRootManifestAndLock(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	var importers []importer = []importer{newGlideImporter(a.loggers, a.sm)}
	for _, i := range importers {
		if i.HasConfig(dir) {
			tool, _ := i.Info()
			if a.loggers.Verbose {
				a.loggers.Err.Printf("Importing %s configuration for %s. Run with -skip-tools to skip.", tool, pr)
			}
			return i.DeriveRootManifestAndLock(dir, pr)
		}
	}

	return nil, nil, nil
}

func (a importAnalyzer) DeriveManifestAndLock(dir string, pr gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	// Ignore other tools if we find dep configuration
	var depAnalyzer dep.Analyzer
	if depAnalyzer.HasConfig(dir) {
		return depAnalyzer.DeriveManifestAndLock(dir, pr)
	}

	var importers []importer = []importer{newGlideImporter(a.loggers, a.sm)}
	for _, i := range importers {
		if i.HasConfig(dir) {
			tool, _ := i.Info()
			if a.loggers.Verbose {
				a.loggers.Err.Printf("Importing %s configuration for %s. Run with -skip-tools to skip.", tool, pr)
			}
			return i.DeriveManifestAndLock(dir, pr)
		}
	}

	return nil, nil, nil
}

func (a importAnalyzer) PostSolveShenanigans(m *dep.Manifest, l *dep.Lock) {
	// do nothing
}
