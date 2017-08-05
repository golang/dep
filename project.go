// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
)

var (
	errProjectNotFound    = fmt.Errorf("could not find project %s, use dep init to initiate a manifest", ManifestName)
	errVendorBackupFailed = fmt.Errorf("failed to create vendor backup. File with same name exists")
)

// findProjectRoot searches from the starting directory upwards looking for a
// manifest file until we get to the root of the filesystem.
func findProjectRoot(from string) (string, error) {
	for {
		mp := filepath.Join(from, ManifestName)

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

// A Project holds a Manifest and optional Lock for a project.
type Project struct {
	// AbsRoot is the absolute path to the root directory of the project.
	AbsRoot string
	// ResolvedAbsRoot is the resolved absolute path to the root directory of the project.
	// If AbsRoot is not a symlink, then ResolvedAbsRoot should equal AbsRoot.
	ResolvedAbsRoot string
	// ImportRoot is the import path of the project's root directory.
	ImportRoot gps.ProjectRoot
	Manifest   *Manifest
	Lock       *Lock // Optional
}

// SetRoot sets the project AbsRoot and ResolvedAbsRoot. If root is a not symlink, ResolvedAbsRoot will be set to root.
func (p *Project) SetRoot(root string) error {
	rroot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}

	p.ResolvedAbsRoot, p.AbsRoot = rroot, root
	return nil
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

// BackupVendor looks for existing vendor directory and if it's not empty,
// creates a backup of it to a new directory with the provided suffix.
func BackupVendor(vpath, suffix string) (string, error) {
	// Check if there's a non-empty vendor directory
	vendorExists, err := fs.IsNonEmptyDir(vpath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if vendorExists {
		// vpath is a full filepath. We need to split it to prefix the backup dir
		// with an "_"
		vpathDir, name := filepath.Split(vpath)
		vendorbak := filepath.Join(vpathDir, "_"+name+"-"+suffix)
		// Check if a directory with same name exists
		if _, err = os.Stat(vendorbak); os.IsNotExist(err) {
			// Copy existing vendor to vendor-{suffix}
			if err := fs.CopyDir(vpath, vendorbak); err != nil {
				return "", err
			}
			return vendorbak, nil
		}
		return "", errVendorBackupFailed
	}

	return "", nil
}
