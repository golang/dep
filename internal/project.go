// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/cfg"
	"github.com/pkg/errors"
)

var errProjectNotFound = fmt.Errorf("could not find project %s, use dep init to initiate a manifest", cfg.ManifestName)

func findProjectRootFromWD() (string, error) {
	path, err := os.Getwd()
	if err != nil {
		return "", errors.Errorf("could not get working directory: %s", err)
	}
	return findProjectRoot(path)
}

// findProjectRoot searches from the starting directory upwards looking for a
// manifest file until we get to the root of the filesystem.
func findProjectRoot(from string) (string, error) {
	for {
		mp := filepath.Join(from, cfg.ManifestName)

		_, err := os.Stat(mp)
		if err == nil {
			return from, nil
		}
		if !os.IsNotExist(err) {
			// Some err other than non-existence - return that out
			return "", err
		}

		parent := filepath.Dir(from)
		if parent == from {
			return "", errProjectNotFound
		}
		from = parent
	}
}

type Project struct {
	// AbsRoot is the absolute path to the root directory of the project.
	AbsRoot string
	// ImportRoot is the import path of the project's root directory.
	ImportRoot gps.ProjectRoot
	Manifest   *cfg.Manifest
	Lock       *cfg.Lock
}

// MakeParams is a simple helper to create a gps.SolveParameters without setting
// any nils incorrectly.
func (p *Project) MakeParams() gps.SolveParameters {
	params := gps.SolveParameters{
		RootDir:         p.AbsRoot,
		ProjectAnalyzer: Analyzer{},
	}

	if p.Manifest != nil {
		params.Manifest = p.Manifest
	}

	if p.Lock != nil {
		params.Lock = p.Lock
	}

	return params
}
