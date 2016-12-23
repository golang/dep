// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
	conf          *manifest // the manifest to write
	lock          *lock     // a struct representation of the current lock, if any
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
	var writeConf, writeLock, writeVendor bool

	if gw.conf != nil {
		writeConf = true
	}

	if gw.nl != nil {
		if gw.lock == nil {
			writeLock, writeVendor = true, true
		} else {
			rlf := lockFromInterface(gw.nl)
			// This err really shouldn't occur, but could if we get an unpaired
			// version back from gps somehow
			if err != nil {
				return err
			}
			if !locksAreEquivalent(rlf, gw.lock) {
				writeLock, writeVendor = true, true
			}
		}
	} else if gw.lock != nil {
		writeLock = true
	}

	if !writeConf && !writeLock && !writeVendor {
		// nothing to do
		return nil
	}

	if writeConf && gw.mpath == "" {
		return fmt.Errorf("Must provide a path if writing out a config yaml.")
	}

	if (writeLock || writeVendor) && gw.vendor == "" {
		return fmt.Errorf("Must provide a vendor dir if writing out a lock or vendor dir.")
	}

	if writeVendor && gw.sm == nil {
		return fmt.Errorf("Must provide a SourceManager if writing out a vendor dir.")
	}

	td, err := ioutil.TempDir(os.TempDir(), "glide")
	if err != nil {
		return fmt.Errorf("Error while creating temp dir for vendor directory: %s", err)
	}
	defer os.RemoveAll(td)

	if writeConf {
		if err := gw.conf.WriteFile(filepath.Join(td, "glide.yaml")); err != nil {
			return fmt.Errorf("Failed to write glide YAML file: %s", err)
		}
	}

	if writeLock {
		if gw.nl == nil {
			// the result lock is nil but the flag is on, so we must be writing
			// the other one
			if err := gw.lock.WriteFile(filepath.Join(td, gpath.LockFile)); err != nil {
				return fmt.Errorf("Failed to write glide lock file: %s", err)
			}
		} else {
			rlf, err := lockFromInterface(gw.nl)
			// As with above, this case really shouldn't get hit unless there's
			// a bug in gps, or guarantees change
			if err != nil {
				return err
			}
			if err := rlf.WriteFile(filepath.Join(td, gpath.LockFile)); err != nil {
				return fmt.Errorf("Failed to write glide lock file: %s", err)
			}
		}
	}

	if writeVendor {
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

	if writeConf {
		if _, err := os.Stat(gw.mpath); err == nil {
			// move out the old one
			tmploc := filepath.Join(td, "glide.yaml-old")
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

	if !fail && writeLock {
		tgt := filepath.Join(filepath.Dir(gw.vendor), gpath.LockFile)
		if _, err := os.Stat(tgt); err == nil {
			// move out the old one
			tmploc := filepath.Join(td, "glide.lock-old")

			failerr = os.Rename(tgt, tmploc)
			if failerr != nil {
				fail = true
			} else {
				restore = append(restore, pathpair{from: tmploc, to: tgt})
			}
		}

		// move in the new one
		failerr = os.Rename(filepath.Join(td, gpath.LockFile), tgt)
		if failerr != nil {
			fail = true
		}
	}

	// have to declare out here so it's present later
	var vendorbak string
	if !fail && writeVendor {
		if _, err := os.Stat(gw.vendor); err == nil {
			// move out the old vendor dir. just do it into an adjacent dir, in
			// order to mitigate the possibility of a pointless cross-filesystem move
			vendorbak = gw.vendor + "-old"
			if _, err := os.Stat(vendorbak); err == nil {
				// Just in case that happens to exist...
				vendorbak = filepath.Join(td, "vendor-old")
			}
			failerr = os.Rename(gw.vendor, vendorbak)
			if failerr != nil {
				fail = true
			} else {
				restore = append(restore, pathpair{from: vendorbak, to: gw.vendor})
			}
		}

		// move in the new one
		failerr = os.Rename(filepath.Join(td, "vendor"), gw.vendor)
		if failerr != nil {
			fail = true
		}
	}

	// If we failed at any point, move all the things back into place, then bail
	if fail {
		for _, pair := range restore {
			// Nothing we can do on err here, we're already in recovery mode
			os.Rename(pair.from, pair.to)
		}
		return failerr
	}

	// Renames all went smoothly. The deferred os.RemoveAll will get the temp
	// dir, but if we wrote vendor, we have to clean that up directly

	if writeVendor {
		// Again, kinda nothing we can do about an error at this point
		os.RemoveAll(vendorbak)
	}

	return nil
}
