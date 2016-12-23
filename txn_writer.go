// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

// safeWriter transactionalizes writes of manifest, lock, and vendor dir, both
// individually and in any combination, into a pseudo-atomic action with
// transactional rollback.
//
// It is not impervious to errors (writing to disk is hard), but it should
// guard against non-arcane failure conditions.
type safeWriter struct {
	root          string
	nm            *manifest // the new manifest to write
	lock          *lock     // the old lock, if any
	nl            gps.Lock  // the new lock to write, if desired
	sm            gps.SourceManager
	mpath, vendor string
}

// writeAllSafe writes out some combination of config yaml, lock, and a vendor
// tree, to a temp dir, then moves them into place if and only if all the write
// operations succeeded. It also does its best to roll back if any moves fail.
//
// This mostly guarantees that dep cannot terminate with a partial write,
// resulting in an undefined disk state.
//
// - If a gw.conf is provided, it will be written to the standard manifest file
// name beneath gw.pr
// - If gw.lock is provided without a gw.nl, it will be written to
//   `glide.lock` in the parent dir of gw.vendor
// - If gw.lock and gw.nl are both provided and are not equivalent,
//   the nl will be written to the same location as above, and a vendor
//   tree will be written to gw.vendor
// - If gw.nl is provided and gw.lock is not, it will write both a lock
//   and vendor dir in the same way
//
// Any of the conf, lock, or result can be omitted; the grouped write operation
// will continue for whichever inputs are present.
func (gw safeWriter) writeAllSafe() error {
	// Decide which writes we need to do
	var writeM, writeL, writeV bool

	if gw.nm != nil {
		writeM = true
	}

	if gw.nl != nil {
		if gw.lock == nil {
			writeL, writeV = true, true
		} else {
			rlf := lockFromInterface(gw.nl)
			if !locksAreEquivalent(rlf, gw.lock) {
				writeL, writeV = true, true
			}
		}
	} else if gw.lock != nil {
		writeL = true
	}

	if !writeM && !writeL && !writeV {
		// nothing to do
		return nil
	}

	if writeM && gw.mpath == "" {
		return fmt.Errorf("Must provide a path if writing out a config yaml.")
	}

	if (writeL || writeV) && gw.vendor == "" {
		return fmt.Errorf("Must provide a vendor dir if writing out a lock or vendor dir.")
	}

	if writeV && gw.sm == nil {
		return fmt.Errorf("Must provide a SourceManager if writing out a vendor dir.")
	}

	td, err := ioutil.TempDir(os.TempDir(), "dep")
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir for writing manifest/lock/vendor")
	}
	defer os.RemoveAll(td)

	if writeM {
		if err := writeFile(filepath.Join(td, manifestName), gw.nm); err != nil {
			return errors.Wrap(err, "failed to write manifest file to temp dir")
		}
	}

	if writeL {
		if gw.nl == nil {
			// the new lock is nil but the flag is on, so we must be writing
			// the other one
			if err := writeFile(filepath.Join(td, lockName), gw.lock); err != nil {
				return errors.Wrap(err, "failed to write lock file to temp dir")
			}
		} else {
			rlf := lockFromInterface(gw.nl)
			// As with above, this case really shouldn't get hit unless there's
			// a bug in gps, or guarantees change
			if err != nil {
				return err
			}
			if err := writeFile(filepath.Join(td, lockName), rlf); err != nil {
				return errors.Wrap(err, "failed to write lock file to temp dir")
			}
		}
	}

	if writeV {
		err = gps.WriteDepTree(filepath.Join(td, "vendor"), gw.nl, gw.sm, true)
		if err != nil {
			return fmt.Errorf("Error while generating vendor tree: %s", err)
		}
	}

	// Move the existing files and dirs to the temp dir while we put the new
	// ones in, to provide insurance against errors for as long as possible
	var fail bool
	var failerr error
	type pathpair struct {
		from, to string
	}
	var restore []pathpair

	if writeM {
		if _, err := os.Stat(gw.mpath); err == nil {
			// move out the old one
			tmploc := filepath.Join(td, manifestName+".orig")
			failerr = os.Rename(gw.mpath, tmploc)
			if failerr != nil {
				fail = true
			} else {
				restore = append(restore, pathpair{from: tmploc, to: gw.mpath})
			}
		}

		// move in the new one
		failerr = os.Rename(filepath.Join(td, manifestName), gw.mpath)
		if failerr != nil {
			fail = true
		}
	}

	if !fail && writeL {
		tgt := filepath.Join(filepath.Dir(gw.vendor), lockName)
		if _, err := os.Stat(tgt); err == nil {
			// move out the old one
			tmploc := filepath.Join(td, lockName+".orig")

			failerr = os.Rename(tgt, tmploc)
			if failerr != nil {
				fail = true
			} else {
				restore = append(restore, pathpair{from: tmploc, to: tgt})
			}
		}

		// move in the new one
		failerr = renameElseCopy(filepath.Join(td, lockName), tgt)
		if failerr != nil {
			fail = true
		}
	}

	// have to declare out here so it's present later
	var vendorbak string
	if !fail && writeV {
		if _, err := os.Stat(gw.vendor); err == nil {
			// move out the old vendor dir. just do it into an adjacent dir, to
			// try to mitigate the possibility of a pointless cross-filesystem
			// move with a temp dir
			vendorbak = gw.vendor + ".orig"
			if _, err := os.Stat(vendorbak); err == nil {
				// If that does already exist bite the bullet and use a proper
				// tempdir
				vendorbak = filepath.Join(td, "vendor.orig")
			}

			failerr = renameElseCopy(gw.vendor, vendorbak)
			if failerr != nil {
				fail = true
			} else {
				restore = append(restore, pathpair{from: vendorbak, to: gw.vendor})
			}
		}

		// move in the new one
		failerr = renameElseCopy(filepath.Join(td, "vendor"), gw.vendor)
		if failerr != nil {
			fail = true
		}
	}

	// If we failed at any point, move all the things back into place, then bail
	if fail {
		for _, pair := range restore {
			// Nothing we can do on err here, we're already in recovery mode
			renameElseCopy(pair.from, pair.to)
		}
		return failerr
	}

	// Renames all went smoothly. The deferred os.RemoveAll will get the temp
	// dir, but if we wrote vendor, we have to clean that up directly

	if writeV {
		// Again, kinda nothing we can do about an error at this point
		os.RemoveAll(vendorbak)
	}

	return nil
}
