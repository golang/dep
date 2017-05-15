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
	Import(path string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error)
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

func (a importAnalyzer) importManifestAndLock(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	importers := []importer{
		newGlideImporter(a.loggers, a.sm),
	}

	for _, i := range importers {
		if i.HasConfig(dir) {
			if a.loggers.Verbose {
				a.loggers.Err.Printf("Importing %T configuration for %s. Run with -skip-tools to skip.", i, pr)
			}
			return i.Import(dir, pr)
		}
	}

	return nil, nil, nil
}

func (a importAnalyzer) DeriveRootManifestAndLock(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	return a.importManifestAndLock(dir, pr)
}

func (a importAnalyzer) DeriveManifestAndLock(dir string, pr gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	// Ignore other tools if we find dep configuration
	var depAnalyzer dep.Analyzer
	if depAnalyzer.HasConfig(dir) {
		return depAnalyzer.DeriveManifestAndLock(dir, pr)
	}

	// The assignment back to an interface prevents interface-based nil checks from failing later
	var manifest gps.Manifest
	var lock gps.Lock
	im, il, err := a.importManifestAndLock(dir, pr)
	if im != nil {
		manifest = im
	}
	if il != nil {
		lock = il
	}
	return manifest, lock, err
}

func (a importAnalyzer) FinalizeManifestAndLock(m *dep.Manifest, l *dep.Lock) {
	// do nothing
}

func (a importAnalyzer) Info() (string, int) {
	// TODO(carolynvs): do not merge until this is set to something unique.
	// I'm not changing it now because that will cause the memo to change in tests
	// which I'll deal with and update later
	return "dep", 1
}
