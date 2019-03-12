// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/pkgtree"
	"github.com/golang/dep/gps/verify"
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
	// The Manifest, as read from Gopkg.toml on disk.
	Manifest *Manifest
	// The Lock, as read from Gopkg.lock on disk.
	Lock *Lock // Optional
	// The above Lock, with changes applied to it. There are two possible classes of
	// changes:
	//  1. Changes to InputImports
	//  2. Changes to per-project prune options
	ChangedLock *Lock
	// The PackageTree representing the project, with hidden and ignored
	// packages already trimmed.
	RootPackageTree pkgtree.PackageTree
	// Oncer to manage access to initial check of vendor.
	CheckVendor sync.Once
	// The result of calling verify.CheckDepTree against the current lock and
	// vendor dir.
	VendorStatus map[string]verify.VendorStatus
	// The error, if any, from checking vendor.
	CheckVendorErr error
}

// VerifyVendor checks the vendor directory against the hash digests in
// Gopkg.lock.
//
// This operation is overseen by the sync.Once in CheckVendor. This is intended
// to facilitate running verification in the background while solving, then
// having the results ready later.
func (p *Project) VerifyVendor() (map[string]verify.VendorStatus, error) {
	p.CheckVendor.Do(func() {
		p.VendorStatus = make(map[string]verify.VendorStatus)
		vendorDir := filepath.Join(p.AbsRoot, "vendor")

		var lps []gps.LockedProject
		if p.Lock != nil {
			lps = p.Lock.Projects()
		}

		sums := make(map[string]verify.VersionedDigest)
		for _, lp := range lps {
			sums[string(lp.Ident().ProjectRoot)] = lp.(verify.VerifiableProject).Digest
		}

		p.VendorStatus, p.CheckVendorErr = verify.CheckDepTree(vendorDir, sums)
	})

	return p.VendorStatus, p.CheckVendorErr
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
		RootPackageTree: p.RootPackageTree,
	}

	if p.Manifest != nil {
		params.Manifest = p.Manifest
	}

	// It should be impossible for p.ChangedLock to be nil if p.Lock is non-nil;
	// we always want to use the former for solving.
	if p.ChangedLock != nil {
		params.Lock = p.ChangedLock
	}

	return params
}

// parseRootPackageTree analyzes the root project's disk contents to create a
// PackageTree, trimming out packages that are not relevant for root projects
// along the way.
//
// The resulting tree is cached internally at p.RootPackageTree.
func (p *Project) parseRootPackageTree() (pkgtree.PackageTree, error) {
	if p.RootPackageTree.Packages == nil {
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
		p.RootPackageTree = ptree.TrimHiddenPackages(true, true, ig)
	}
	return p.RootPackageTree, nil
}

// GetDirectDependencyNames returns the set of unique Project Roots that are the
// direct dependencies of this Project.
//
// A project is considered a direct dependency if at least one of its packages
// is named in either this Project's required list, or if there is at least one
// non-ignored import statement from a non-ignored package in the current
// project's package tree.
//
// The returned map of Project Roots contains only boolean true values; this
// makes a "false" value always indicate an absent key, which makes conditional
// checks against the map more ergonomic.
//
// This function will correctly utilize ignores and requireds from an existing
// manifest, if one is present, but will also do the right thing without a
// manifest.
func (p *Project) GetDirectDependencyNames(ctx context.Context, sm gps.SourceManager) (map[gps.ProjectRoot]bool, error) {
	var reach []string
	if p.ChangedLock != nil {
		reach = p.ChangedLock.InputImports()
	} else {
		ptree, err := p.parseRootPackageTree()
		if err != nil {
			return nil, err
		}
		reach = externalImportList(ptree, p.Manifest)
	}

	directDeps := map[gps.ProjectRoot]bool{}
	for _, ip := range reach {
		pr, err := sm.DeduceProjectRoot(ctx, ip)
		if err != nil {
			return nil, err
		}
		directDeps[pr] = true
	}

	return directDeps, nil
}

// FindIneffectualConstraints looks for constraint rules expressed in the
// manifest that will have no effect during solving, as they are specified for
// projects that are not direct dependencies of the Project.
//
// "Direct dependency" here is as implemented by GetDirectDependencyNames();
// it correctly incorporates all "ignored" and "required" rules.
func (p *Project) FindIneffectualConstraints(ctx context.Context, sm gps.SourceManager) []gps.ProjectRoot {
	if p.Manifest == nil {
		return nil
	}

	dd, err := p.GetDirectDependencyNames(ctx, sm)
	if err != nil {
		return nil
	}

	var ineff []gps.ProjectRoot
	for pr := range p.Manifest.DependencyConstraints() {
		if !dd[pr] {
			ineff = append(ineff, pr)
		}
	}

	sort.Slice(ineff, func(i, j int) bool {
		return ineff[i] < ineff[j]
	})
	return ineff
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
