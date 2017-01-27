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
	Root          string    // absolute path of root dir in which to write
	Manifest      *Manifest // the manifest to write, if any
	Lock          *Lock     // the old lock, if any
	NewLock       gps.Lock  // the new lock, if any
	SourceManager gps.SourceManager
}

// WriteAllSafe writes out some combination of config yaml, lock, and a vendor
// tree, to a temp dir, then moves them into place if and only if all the write
// operations succeeded. It also does its best to roll back if any moves fail.
//
// This mostly guarantees that dep cannot exit with a partial write that would
// leave an undefined state on disk.
//
// - If a sw.Manifest is provided, it will be written to the standard manifest file
//   name beneath sw.Root
// - If sw.Lock is provided without an sw.NewLock, it will be written to the standard
//   lock file name in the root dir, but vendor will NOT be written
// - If sw.Lock and sw.NewLock are both provided and are equivalent, then neither lock
//   nor vendor will be written
// - If sw.Lock and sw.NewLock are both provided and are not equivalent,
//   the nl will be written to the same location as above, and a vendor
//   tree will be written to sw.Root/vendor
// - If sw.NewLock is provided and sw.Lockock is not, it will write both a lock
//   and vendor dir in the same way
// - If the forceVendor param is true, then vendor will be unconditionally
//   written out based on sw.NewLock if present, else sw.Lock, else error.
//
// Any of m, l, or nl can be omitted; the grouped write operation will continue
// for whichever inputs are present. A SourceManager is only required if vendor
// is being written.
func (sw SafeWriter) WriteAllSafe(forceVendor bool) error {
	// Decide which writes we need to do
	var writeM, writeL, writeV bool
	writeV = forceVendor

	if sw.Manifest != nil {
		writeM = true
	}

	if sw.NewLock != nil {
		if sw.Lock == nil {
			writeL, writeV = true, true
		} else {
			rlf := LockFromInterface(sw.NewLock)
			if !locksAreEquivalent(rlf, sw.Lock) {
				writeL, writeV = true, true
			}
		}
	} else if sw.Lock != nil {
		writeL = true
	}

	if sw.Root == "" {
		return errors.New("root path must be non-empty")
	}
	if is, err := IsDir(sw.Root); !is {
		if err != nil {
			return err
		}
		return fmt.Errorf("root path %q does not exist", sw.Root)
	}

	if !writeM && !writeL && !writeV {
		// nothing to do
		return nil
	}

	if writeV && sw.SourceManager == nil {
		return errors.New("must provide a SourceManager if writing out a vendor dir")
	}

	if writeV && sw.Lock == nil && sw.NewLock == nil {
		return errors.New("must provide a lock in order to write out vendor")
	}

	mpath := filepath.Join(sw.Root, ManifestName)
	lpath := filepath.Join(sw.Root, LockName)
	vpath := filepath.Join(sw.Root, "vendor")

	td, err := ioutil.TempDir(os.TempDir(), "dep")
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir for writing manifest/lock/vendor")
	}
	defer os.RemoveAll(td)

	if writeM {
		if err := writeFile(filepath.Join(td, ManifestName), sw.Manifest); err != nil {
			return errors.Wrap(err, "failed to write manifest file to temp dir")
		}
	}

	if writeL {
		if sw.NewLock == nil {
			// the new lock is nil but the flag is on, so we must be writing
			// the other one
			if err := writeFile(filepath.Join(td, LockName), sw.Lock); err != nil {
				return errors.Wrap(err, "failed to write lock file to temp dir")
			}
		} else {
			rlf := LockFromInterface(sw.NewLock)
			if err := writeFile(filepath.Join(td, LockName), rlf); err != nil {
				return errors.Wrap(err, "failed to write lock file to temp dir")
			}
		}
	}

	if writeV {
		// Prefer the nl, but take the l if only that's available, as could be the
		// case if true was passed for forceVendor.
		l := sw.NewLock
		if l == nil {
			l = sw.Lock
		}
		err = gps.WriteDepTree(filepath.Join(td, "vendor"), l, sw.SourceManager, true)
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

	if writeM {
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

	if writeL {
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

	if writeV {
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
	if writeV {
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
