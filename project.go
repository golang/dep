// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/pkgtree"
	"github.com/golang/dep/internal/fs"
	"github.com/pkg/errors"
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

// checkGopkgFilenames validates filename case for the manifest and lock files.
//
// This is relevant on case-insensitive file systems like the defaults in Windows and
// macOS.
//
// If manifest file is not found, it returns an error indicating the project could not be
// found. If it is found but the case does not match, an error is returned. If a lock
// file is not found, no error is returned as lock file is optional. If it is found but
// the case does not match, an error is returned.
func checkGopkgFilenames(projectRoot string) error {
	// ReadActualFilenames is actually costly. Since the check to validate filename case
	// for Gopkg filenames is not relevant to case-sensitive filesystems like
	// ext4(linux), try for an early return.
	caseSensitive, err := fs.IsCaseSensitiveFilesystem(projectRoot)
	if err != nil {
		return errors.Wrap(err, "could not check validity of configuration filenames")
	}
	if caseSensitive {
		return nil
	}

	actualFilenames, err := fs.ReadActualFilenames(projectRoot, []string{ManifestName, LockName})

	if err != nil {
		return errors.Wrap(err, "could not check validity of configuration filenames")
	}

	actualMfName, found := actualFilenames[ManifestName]
	if !found {
		// Ideally this part of the code won't ever be executed if it is called after
		// `findProjectRoot`. But be thorough and handle it anyway.
		return errProjectNotFound
	}
	if actualMfName != ManifestName {
		return fmt.Errorf("manifest filename %q does not match %q", actualMfName, ManifestName)
	}

	// If a file is not found, the string map returned by `fs.ReadActualFilenames` will
	// not have an entry for the given filename. Since the lock file is optional, we
	// should check for equality only if it was found.
	actualLfName, found := actualFilenames[LockName]
	if found && actualLfName != LockName {
		return fmt.Errorf("lock filename %q does not match %q", actualLfName, LockName)
	}

	return nil
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

// SetRoot sets the project AbsRoot and ResolvedAbsRoot. If root is not a symlink, ResolvedAbsRoot will be set to root.
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

// ParseRootPackageTree analyzes the root project's disk contents to create a
// PackageTree, trimming out packages that are not relevant for root projects
// along the way.
func (p *Project) ParseRootPackageTree() (pkgtree.PackageTree, error) {
	ptree, err := pkgtree.ListPackages(p.ResolvedAbsRoot, string(p.ImportRoot))
	if err != nil {
		return pkgtree.PackageTree{}, errors.Wrap(err, "analysis of current project's packages failed")
	}
	// We don't care about (unreachable) hidden packages for the root project,
	// so drop all of those.
	var ig *pkgtree.IgnoredRuleset
	if p.Manifest != nil {
		ig = p.Manifest.IgnoredPackages()
	}
	return ptree.TrimHiddenPackages(true, true, ig), nil
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
