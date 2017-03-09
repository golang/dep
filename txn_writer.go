// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

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
	Manifest         *Manifest
	Lock             *Lock
	LockDiff         *LockDiff
	ForceWriteVendor bool
}

func (payload *SafeWriterPayload) HasLock() bool {
	return payload.Lock != nil
}

func (payload *SafeWriterPayload) HasManifest() bool {
	return payload.Manifest != nil
}

func (payload *SafeWriterPayload) HasVendor() bool {
	// TODO(carolynvs) this can be calculated based on if we are writing the lock
	// init -> switch to newlock
	// ensure checks existence, why not move that into the prep?
	return payload.ForceWriteVendor
}

// LockDiff is the set of differences between an existing lock file and an updated lock file.
// TODO(carolynvs) this should be moved to gps
type LockDiff struct {
	Add    []gps.LockedProject
	Remove []gps.LockedProject
	Modify []LockedProjectDiff
}

// LockedProjectDiff contains the before and after snapshot of a project reference.
// TODO(carolynvs) this should be moved to gps
type LockedProjectDiff struct {
	Current gps.LockedProject // Current represents the project reference as defined in the existing lock file.
	Updated gps.LockedProject // Updated represents the desired project reference.
}

// Prepare to write a set of config yaml, lock and vendor tree.
//
// - If manifest is provided, it will be written to the standard manifest file
//   name beneath root.
// - If lock is provided it will be written to the standard
//   lock file name in the root dir, but vendor will NOT be written
// - If lock and newLock are both provided and are equivalent, then neither lock
//   nor vendor will be written
// - If lock and newLock are both provided and are not equivalent,
//   the newLock will be written to the same location as above, and a vendor
//   tree will be written to the vendor directory
// - If newLock is provided and lock is not, it will write both a lock
//   and the vendor directory in the same way
// - If the forceVendor param is true, then vendor/ will be unconditionally
//   written out based on newLock if present, else lock, else error.
func (sw *SafeWriter) Prepare(manifest *Manifest, lock *Lock, newLock gps.Lock, forceVendor bool) {
	sw.Payload = &SafeWriterPayload{
		Manifest:         manifest,
		ForceWriteVendor: forceVendor,
	}

	if newLock != nil {
		rlf := LockFromInterface(newLock)
		if lock == nil {
			sw.Payload.Lock = rlf
			sw.Payload.ForceWriteVendor = true
		} else {
			if !locksAreEquivalent(rlf, lock) {
				sw.Payload.Lock = rlf
				sw.Payload.ForceWriteVendor = true
			}
		}
	} else if lock != nil {
		sw.Payload.Lock = lock
	}
}

func (payload SafeWriterPayload) validate(root string, sm gps.SourceManager) error {
	if root == "" {
		return errors.New("root path must be non-empty")
	}
	if is, err := IsDir(root); !is {
		if err != nil {
			return err
		}
		return fmt.Errorf("root path %q does not exist", root)
	}

	if payload.HasVendor() && sm == nil {
		return errors.New("must provide a SourceManager if writing out a vendor dir")
	}

	if payload.HasVendor() && payload.Lock == nil {
		return errors.New("must provide a lock in order to write out vendor")
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
func (sw *SafeWriter) Write(root string, sm gps.SourceManager) error {
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
		if err := writeFile(filepath.Join(td, ManifestName), sw.Payload.Manifest); err != nil {
			return errors.Wrap(err, "failed to write manifest file to temp dir")
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
		fmt.Println("Would have written the following manifest.json:")
		m, err := sw.Payload.Manifest.MarshalJSON()
		if err != nil {
			return errors.Wrap(err, "ensure DryRun cannot read manifest")
		}
		fmt.Println(string(m))
	}

	if sw.Payload.HasLock() {
		fmt.Println("Would have written the following lock.json:")
		m, err := sw.Payload.Lock.MarshalJSON()
		if err != nil {
			return errors.Wrap(err, "ensure DryRun cannot read lock")
		}
		fmt.Println(string(m))
	}

	if sw.Payload.HasVendor() {
		fmt.Println("Would have written the following projects to the vendor directory:")
		for _, project := range sw.Payload.Lock.Projects() {
			prj := project.Ident()
			rev := GetRevisionFromVersion(project.Version())
			if prj.Source == "" {
				fmt.Printf("%s@%s\n", prj.ProjectRoot, rev)
			} else {
				fmt.Printf("%s -> %s@%s\n", prj.ProjectRoot, prj.Source, rev)
			}
		}
	}

	return nil
}
