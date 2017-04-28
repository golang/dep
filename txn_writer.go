// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

// Example string to be written to the manifest file
// if no dependencies are found in the project
// during `dep init`
const exampleTOML = `
## Gopkg.toml example (these lines may be deleted)

## "required" lists a set of packages (not projects) that must be included in
## Gopkg.lock. This list is merged with the set of packages imported by the current
## project. Use it when your project needs a package it doesn't explicitly import -
## including "main" packages.
# required = ["github.com/user/thing/cmd/thing"]

## "ignored" lists a set of packages (not projects) that are ignored when
## dep statically analyzes source code. Ignored packages can be in this project,
## or in a dependency.
# ignored = ["github.com/user/project/badpkg"]

## Dependencies define constraints on dependent projects. They are respected by
## dep whether coming from the Gopkg.toml of the current project or a dependency.
# [[dependencies]]
## Required: the root import path of the project being constrained.
# name = "github.com/user/project"
#
## Recommended: the version constraint to enforce for the project.
## Only one of "branch", "version" or "revision" can be specified.
# version = "1.0.0"
# branch = "master"
# revision = "abc123"
#
## Optional: an alternate location (URL or import path) for the project's source.
# source = "https://github.com/myfork/package.git"

## Overrides have the same structure as [[dependencies]], but supercede all
## [[dependencies]] declarations from all projects. Only the current project's
## [[overrides]] are applied.
##
## Overrides are a sledgehammer. Use them only as a last resort.
# [[overrides]]
## Required: the root import path of the project being constrained.
# name = "github.com/user/project"
#
## Optional: specifying a version constraint override will cause all other
## constraints on this project to be ignored; only the overriden constraint
## need be satisfied.
## Again, only one of "branch", "version" or "revision" can be specified.
# version = "1.0.0"
# branch = "master"
# revision = "abc123"
#
## Optional: specifying an alternate source location as an override will
## enforce that the alternate location is used for that project, regardless of
## what source location any dependent projects specify.
# source = "https://github.com/myfork/package.git"


`

// SafeWriter transactionalizes writes of manifest, lock, and vendor dir, both
// individually and in any combination, into a pseudo-atomic action with
// transactional rollback.
//
// It is not impervious to errors (writing to disk is hard), but it should
// guard against non-arcane failure conditions.
type SafeWriter struct {
	Payload *SafeWriterPayload
}

// SafeWriterPayload represents the actions SafeWriter will execute when SafeWriter.Write is called.
type SafeWriterPayload struct {
	Manifest    *Manifest
	Lock        *Lock
	LockDiff    *gps.LockDiff
	WriteVendor bool
}

func (payload *SafeWriterPayload) HasLock() bool {
	return payload.Lock != nil
}

func (payload *SafeWriterPayload) HasManifest() bool {
	return payload.Manifest != nil
}

func (payload *SafeWriterPayload) HasVendor() bool {
	return payload.WriteVendor
}

type rawStringDiff struct {
	*gps.StringDiff
}

func (diff rawStringDiff) MarshalTOML() ([]byte, error) {
	return []byte(diff.String()), nil
}

type rawLockDiff struct {
	*gps.LockDiff
}

type rawLockedProjectDiff struct {
	Name     gps.ProjectRoot `toml:"name"`
	Source   *rawStringDiff  `toml:"source,omitempty"`
	Version  *rawStringDiff  `toml:"version,omitempty"`
	Branch   *rawStringDiff  `toml:"branch,omitempty"`
	Revision *rawStringDiff  `toml:"revision,omitempty"`
	Packages []rawStringDiff `toml:"packages,omitempty"`
}

func toRawLockedProjectDiff(diff gps.LockedProjectDiff) rawLockedProjectDiff {
	// this is a shallow copy since we aren't modifying the raw diff
	raw := rawLockedProjectDiff{Name: diff.Name}
	if diff.Source != nil {
		raw.Source = &rawStringDiff{diff.Source}
	}
	if diff.Version != nil {
		raw.Version = &rawStringDiff{diff.Version}
	}
	if diff.Branch != nil {
		raw.Branch = &rawStringDiff{diff.Branch}
	}
	if diff.Revision != nil {
		raw.Revision = &rawStringDiff{diff.Revision}
	}
	raw.Packages = make([]rawStringDiff, len(diff.Packages))
	for i := 0; i < len(diff.Packages); i++ {
		raw.Packages[i] = rawStringDiff{&diff.Packages[i]}
	}
	return raw
}

type rawLockedProjectDiffs struct {
	Projects []rawLockedProjectDiff `toml:"projects"`
}

func toRawLockedProjectDiffs(diffs []gps.LockedProjectDiff) rawLockedProjectDiffs {
	raw := rawLockedProjectDiffs{
		Projects: make([]rawLockedProjectDiff, len(diffs)),
	}

	for i := 0; i < len(diffs); i++ {
		raw.Projects[i] = toRawLockedProjectDiff(diffs[i])
	}

	return raw
}

func formatLockDiff(diff gps.LockDiff) (string, error) {
	var buf bytes.Buffer

	if diff.HashDiff != nil {
		buf.WriteString(fmt.Sprintf("Memo: %s\n\n", diff.HashDiff))
	}

	writeDiffs := func(diffs []gps.LockedProjectDiff) error {
		raw := toRawLockedProjectDiffs(diffs)
		chunk, err := toml.Marshal(raw)
		if err != nil {
			return err
		}
		buf.Write(chunk)
		buf.WriteString("\n")
		return nil
	}

	if len(diff.Add) > 0 {
		buf.WriteString("Add:")
		err := writeDiffs(diff.Add)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Add")
		}
	}

	if len(diff.Remove) > 0 {
		buf.WriteString("Remove:")
		err := writeDiffs(diff.Remove)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Remove")
		}
	}

	if len(diff.Modify) > 0 {
		buf.WriteString("Modify:")
		err := writeDiffs(diff.Modify)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Modify")
		}
	}

	return buf.String(), nil
}

// VendorBehavior defines when the vendor directory should be written.
type VendorBehavior int

const (
	// VendorOnChanged indicates that the vendor directory should be written when the lock is new or changed.
	VendorOnChanged VendorBehavior = iota
	// VendorAlways forces the vendor directory to always be written.
	VendorAlways
	// VendorNever indicates the vendor directory should never be written.
	VendorNever
)

// Prepare to write a set of config yaml, lock and vendor tree.
//
// - If manifest is provided, it will be written to the standard manifest file
//   name beneath root.
// - If newLock is provided, it will be written to the standard lock file
//   name beneath root.
// - If vendor is VendorAlways, or is VendorOnChanged and the locks are different,
//   the vendor directory will be written beneath root based on newLock.
// - If oldLock is provided without newLock, error.
// - If vendor is VendorAlways without a newLock, error.
func (sw *SafeWriter) Prepare(manifest *Manifest, oldLock, newLock *Lock, vendor VendorBehavior) error {

	sw.Payload = &SafeWriterPayload{
		Manifest: manifest,
		Lock:     newLock,
	}

	if oldLock != nil {
		if newLock == nil {
			return errors.New("must provide newLock when oldLock is specified")
		}
		sw.Payload.LockDiff = gps.DiffLocks(oldLock, newLock)
	}

	switch vendor {
	case VendorAlways:
		sw.Payload.WriteVendor = true
	case VendorOnChanged:
		if sw.Payload.LockDiff != nil || (newLock != nil && oldLock == nil) {
			sw.Payload.WriteVendor = true
		}
	}

	if sw.Payload.WriteVendor && newLock == nil {
		return errors.New("must provide newLock in order to write out vendor")
	}

	return nil
}

func (payload SafeWriterPayload) validate(root string, sm gps.SourceManager) error {
	if root == "" {
		return errors.New("root path must be non-empty")
	}
	if is, err := IsDir(root); !is {
		if err != nil {
			return err
		}
		return errors.Errorf("root path %q does not exist", root)
	}

	if payload.HasVendor() && sm == nil {
		return errors.New("must provide a SourceManager if writing out a vendor dir")
	}

	return nil
}

// Write saves some combination of config yaml, lock, and a vendor tree.
// root is the absolute path of root dir in which to write.
// sm is only required if vendor is being written.
//
// It first writes to a temp dir, then moves them in place if and only if all the write
// operations succeeded. It also does its best to roll back if any moves fail.
// This mostly guarantees that dep cannot exit with a partial write that would
// leave an undefined state on disk.
func (sw *SafeWriter) Write(root string, sm gps.SourceManager, noExamples bool) error {

	if sw.Payload == nil {
		return errors.New("Cannot call SafeWriter.Write before SafeWriter.Prepare")
	}

	err := sw.Payload.validate(root, sm)
	if err != nil {
		return err
	}

	if !sw.Payload.HasManifest() && !sw.Payload.HasLock() && !sw.Payload.HasVendor() {
		// nothing to do
		return nil
	}

	mpath := filepath.Join(root, ManifestName)
	lpath := filepath.Join(root, LockName)
	vpath := filepath.Join(root, "vendor")

	td, err := ioutil.TempDir(os.TempDir(), "dep")
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir for writing manifest/lock/vendor")
	}
	defer os.RemoveAll(td)

	if sw.Payload.HasManifest() {
		// Always write the example text to the bottom of the TOML file.
		tb, err := sw.Payload.Manifest.MarshalTOML()
		if err != nil {
			return errors.Wrap(err, "failed to marshal manifest to TOML")
		}

		if !noExamples {
			// 0666 is before umask; mirrors behavior of os.Create (used by
			// writeFile())
			if err = ioutil.WriteFile(filepath.Join(td, ManifestName), append([]byte(exampleTOML), tb...), 0666); err != nil {
				return errors.Wrap(err, "failed to write manifest file to temp dir")
			}
		}
	}

	if sw.Payload.HasLock() {
		if err := writeFile(filepath.Join(td, LockName), sw.Payload.Lock); err != nil {
			return errors.Wrap(err, "failed to write lock file to temp dir")
		}
	}

	if sw.Payload.HasVendor() {
		err = gps.WriteDepTree(filepath.Join(td, "vendor"), sw.Payload.Lock, sm, true)
		if err != nil {
			return errors.Wrap(err, "error while writing out vendor tree")
		}
	}

	// Ensure vendor/.git is preserved if present
	if hasDotGit(vpath) {
		err = renameWithFallback(filepath.Join(vpath, ".git"), filepath.Join(td, "vendor/.git"))
		if _, ok := err.(*os.LinkError); ok {
			return errors.Wrap(err, "failed to preserve vendor/.git")
		}
	}

	// Move the existing files and dirs to the temp dir while we put the new
	// ones in, to provide insurance against errors for as long as possible.
	type pathpair struct {
		from, to string
	}
	var restore []pathpair
	var failerr error
	var vendorbak string

	if sw.Payload.HasManifest() {
		if _, err := os.Stat(mpath); err == nil {
			// Move out the old one.
			tmploc := filepath.Join(td, ManifestName+".orig")
			failerr = renameWithFallback(mpath, tmploc)
			if failerr != nil {
				goto fail
			}
			restore = append(restore, pathpair{from: tmploc, to: mpath})
		}

		// Move in the new one.
		failerr = renameWithFallback(filepath.Join(td, ManifestName), mpath)
		if failerr != nil {
			goto fail
		}
	}

	if sw.Payload.HasLock() {
		if _, err := os.Stat(lpath); err == nil {
			// Move out the old one.
			tmploc := filepath.Join(td, LockName+".orig")

			failerr = renameWithFallback(lpath, tmploc)
			if failerr != nil {
				goto fail
			}
			restore = append(restore, pathpair{from: tmploc, to: lpath})
		}

		// Move in the new one.
		failerr = renameWithFallback(filepath.Join(td, LockName), lpath)
		if failerr != nil {
			goto fail
		}
	}

	if sw.Payload.HasVendor() {
		if _, err := os.Stat(vpath); err == nil {
			// Move out the old vendor dir. just do it into an adjacent dir, to
			// try to mitigate the possibility of a pointless cross-filesystem
			// move with a temp directory.
			vendorbak = vpath + ".orig"
			if _, err := os.Stat(vendorbak); err == nil {
				// If the adjacent dir already exists, bite the bullet and move
				// to a proper tempdir.
				vendorbak = filepath.Join(td, "vendor.orig")
			}

			failerr = renameWithFallback(vpath, vendorbak)
			if failerr != nil {
				goto fail
			}
			restore = append(restore, pathpair{from: vendorbak, to: vpath})
		}

		// Move in the new one.
		failerr = renameWithFallback(filepath.Join(td, "vendor"), vpath)
		if failerr != nil {
			goto fail
		}
	}

	// Renames all went smoothly. The deferred os.RemoveAll will get the temp
	// dir, but if we wrote vendor, we have to clean that up directly
	if sw.Payload.HasVendor() {
		// Nothing we can really do about an error at this point, so ignore it
		os.RemoveAll(vendorbak)
	}

	return nil

fail:
	// If we failed at any point, move all the things back into place, then bail.
	for _, pair := range restore {
		// Nothing we can do on err here, as we're already in recovery mode.
		renameWithFallback(pair.from, pair.to)
	}
	return failerr
}

func (sw *SafeWriter) PrintPreparedActions() error {
	if sw.Payload.HasManifest() {
		fmt.Printf("Would have written the following %s:\n", ManifestName)
		m, err := sw.Payload.Manifest.MarshalTOML()
		if err != nil {
			return errors.Wrap(err, "ensure DryRun cannot serialize manifest")
		}
		fmt.Println(string(m))
	}

	if sw.Payload.HasLock() {
		if sw.Payload.LockDiff == nil {
			fmt.Printf("Would have written the following %s:\n", LockName)
			l, err := sw.Payload.Lock.MarshalTOML()
			if err != nil {
				return errors.Wrap(err, "ensure DryRun cannot serialize lock")
			}
			fmt.Println(string(l))
		} else {
			fmt.Printf("Would have written the following changes to %s:\n", LockName)
			diff, err := formatLockDiff(*sw.Payload.LockDiff)
			if err != nil {
				return errors.Wrap(err, "ensure DryRun cannot serialize the lock diff")
			}
			fmt.Println(diff)
		}
	}

	if sw.Payload.HasVendor() {
		fmt.Println("Would have written the following projects to the vendor directory:")
		for _, project := range sw.Payload.Lock.Projects() {
			prj := project.Ident()
			rev, _, _ := gps.VersionComponentStrings(project.Version())
			if prj.Source == "" {
				fmt.Printf("%s@%s\n", prj.ProjectRoot, rev)
			} else {
				fmt.Printf("%s -> %s@%s\n", prj.ProjectRoot, prj.Source, rev)
			}
		}
	}

	return nil
}

func PruneProject(p *Project, sm gps.SourceManager) error {
	td, err := ioutil.TempDir(os.TempDir(), "dep")
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir for writing manifest/lock/vendor")
	}
	defer os.RemoveAll(td)

	if err := gps.WriteDepTree(td, p.Lock, sm, true); err != nil {
		return err
	}

	var toKeep []string
	for _, project := range p.Lock.Projects() {
		projectRoot := string(project.Ident().ProjectRoot)
		for _, pkg := range project.Packages() {
			toKeep = append(toKeep, filepath.Join(projectRoot, pkg))
		}
	}

	toDelete, err := calculatePrune(td, toKeep)
	if err != nil {
		return err
	}

	if err := deleteDirs(toDelete); err != nil {
		return err
	}

	vpath := filepath.Join(p.AbsRoot, "vendor")
	vendorbak := vpath + ".orig"
	var failerr error
	if _, err := os.Stat(vpath); err == nil {
		// Move out the old vendor dir. just do it into an adjacent dir, to
		// try to mitigate the possibility of a pointless cross-filesystem
		// move with a temp directory.
		if _, err := os.Stat(vendorbak); err == nil {
			// If the adjacent dir already exists, bite the bullet and move
			// to a proper tempdir.
			vendorbak = filepath.Join(td, "vendor.orig")
		}
		failerr = renameWithFallback(vpath, vendorbak)
		if failerr != nil {
			goto fail
		}
	}

	// Move in the new one.
	failerr = renameWithFallback(td, vpath)
	if failerr != nil {
		goto fail
	}

	os.RemoveAll(vendorbak)

	return nil

fail:
	renameWithFallback(vendorbak, vpath)
	return failerr
}

func calculatePrune(vendorDir string, keep []string) ([]string, error) {
	sort.Strings(keep)
	toDelete := []string{}
	err := filepath.Walk(vendorDir, func(path string, info os.FileInfo, err error) error {
		if _, err := os.Lstat(path); err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if path == vendorDir {
			return nil
		}

		name := strings.TrimPrefix(path, vendorDir+"/")
		i := sort.Search(len(keep), func(i int) bool {
			return name <= keep[i]
		})
		if i >= len(keep) || !strings.HasPrefix(keep[i], name) {
			toDelete = append(toDelete, path)
		}
		return nil
	})
	return toDelete, err
}

func deleteDirs(toDelete []string) error {
	// sort by length so we delete sub dirs first
	sort.Sort(byLen(toDelete))
	for _, path := range toDelete {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

// hasDotGit checks if a given path has .git file or directory in it.
func hasDotGit(path string) bool {
	gitfilepath := filepath.Join(path, ".git")
	_, err := os.Stat(gitfilepath)
	return err == nil
}

type byLen []string

func (a byLen) Len() int           { return len(a) }
func (a byLen) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byLen) Less(i, j int) bool { return len(a[i]) > len(a[j]) }
