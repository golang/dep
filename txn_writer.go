// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
// Fields are only populated when there is a difference, otherwise they are empty.
// TODO(carolynvs) this should be moved to gps
type LockDiff struct {
	HashDiff *StringDiff
	Add      []LockedProjectDiff
	Remove   []gps.ProjectRoot
	Modify   []LockedProjectDiff
}

func (diff *LockDiff) Format() (string, error) {
	if diff == nil {
		return "", nil
	}

	var buf bytes.Buffer

	if len(diff.Add) > 0 {
		buf.WriteString("Add: ")

		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "    ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(diff.Add)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Add")
		}
	}

	if len(diff.Remove) > 0 {
		buf.WriteString("Remove: ")

		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "    ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(diff.Remove)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Remove")
		}
	}

	if len(diff.Modify) > 0 {
		buf.WriteString("Modify: ")

		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "    ")
		enc.SetEscapeHTML(false)
		err := enc.Encode(diff.Modify)
		if err != nil {
			return "", errors.Wrap(err, "Unable to format LockDiff.Modify")
		}
	}

	return buf.String(), nil
}

// LockedProjectDiff contains the before and after snapshot of a project reference.
// Fields are only populated when there is a difference, otherwise they are empty.
// TODO(carolynvs) this should be moved to gps
type LockedProjectDiff struct {
	Name       gps.ProjectRoot `json:"name"`
	Source   *StringDiff     `json:"repo,omitempty"`
	Version    *StringDiff     `json:"version,omitempty"`
	Branch     *StringDiff     `json:"branch,omitempty"`
	Revision   *StringDiff     `json:"revision,omitempty"`
	Packages   []StringDiff    `json:"packages,omitempty"`
}

type StringDiff struct {
	Previous string
	Current  string
}

func (diff StringDiff) MarshalJSON() ([]byte, error) {
	var value string

	if diff.Previous == "" && diff.Current != "" {
		value = fmt.Sprintf("+ %s", diff.Current)
	} else if diff.Previous != "" && diff.Current == "" {
		value = fmt.Sprintf("- %s", diff.Previous)
	} else if diff.Previous != diff.Current {
		value = fmt.Sprintf("%s -> %s", diff.Previous, diff.Current)
	} else {
		value = diff.Current
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(value)

	return buf.Bytes(), err
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
func (sw *SafeWriter) Prepare(manifest *Manifest, lock *Lock, newLock *Lock, forceVendor bool) {
	sw.Payload = &SafeWriterPayload{
		Manifest:         manifest,
		ForceWriteVendor: forceVendor,
	}

	if newLock != nil {
		if lock == nil {
			sw.Payload.Lock = newLock
			sw.Payload.ForceWriteVendor = true
		} else {
			diff := diffLocks(lock, newLock)
			if diff != nil {
				sw.Payload.Lock = newLock
				sw.Payload.LockDiff = diff
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
			return errors.Wrap(err, "ensure DryRun cannot serialize manifest")
		}
		fmt.Println(string(m))
	}

	if sw.Payload.HasLock() {
		fmt.Println("Would have written the following changes to lock.json:")
		diff, err := sw.Payload.LockDiff.Format()
		if err != nil {
			return errors.Wrap(err, "ensure DryRun cannot serialize the lock diff")
		}
		fmt.Println(diff)
	}

	if sw.Payload.HasVendor() {
		fmt.Println("Would have written the following projects to the vendor directory:")
		for _, project := range sw.Payload.Lock.Projects() {
			prj := project.Ident()
			rev, _, _ := getVersionInfo(project.Version())
			if prj.Source == "" {
				fmt.Printf("%s@%s\n", prj.ProjectRoot, rev)
			} else {
				fmt.Printf("%s -> %s@%s\n", prj.ProjectRoot, prj.Source, rev)
			}
		}
	}

	return nil
}

// diffLocks compares two locks and identifies the differences between them.
// Returns nil if there are no differences.
// TODO(carolynvs) this should be moved to gps
func diffLocks(l1 gps.Lock, l2 gps.Lock) *LockDiff {
	// Default nil locks to empty locks, so that we can still generate a diff
	if l1 == nil {
		l1 = &gps.SimpleLock{}
	}
	if l2 == nil {
		l2 = &gps.SimpleLock{}
	}

	p1, p2 := l1.Projects(), l2.Projects()

	// Check if the slices are sorted already. If they are, we can compare
	// without copying. Otherwise, we have to copy to avoid altering the
	// original input.
	sp1, sp2 := SortedLockedProjects(p1), SortedLockedProjects(p2)
	if len(p1) > 1 && !sort.IsSorted(sp1) {
		p1 = make([]gps.LockedProject, len(p1))
		copy(p1, l1.Projects())
		sort.Sort(SortedLockedProjects(p1))
	}
	if len(p2) > 1 && !sort.IsSorted(sp2) {
		p2 = make([]gps.LockedProject, len(p2))
		copy(p2, l2.Projects())
		sort.Sort(SortedLockedProjects(p2))
	}

	diff := LockDiff{}

	h1 := l1.InputHash()
	h2 := l2.InputHash()
	if !bytes.Equal(h1, h2) {
		diff.HashDiff = &StringDiff{Previous: string(h1), Current: string(h2)}
	}

	var i2next int
	for i1 := 0; i1 < len(p1); i1++ {
		lp1 := p1[i1]
		pr1 := lp1.Ident().ProjectRoot

		var matched bool
		for i2 := i2next; i2 < len(p2); i2++ {
			lp2 := p2[i2]
			pr2 := lp2.Ident().ProjectRoot

			switch strings.Compare(string(pr1), string(pr2)) {
			case 0: // Found a matching project
				matched = true
				pdiff := diffProjects(lp1, lp2)
				if pdiff != nil {
					diff.Modify = append(diff.Modify, *pdiff)
				}
				i2next = i2 + 1 // Don't evaluate to this again
			case -1: // Found a new project
				add := buildAddProject(lp2)
				diff.Add = append(diff.Add, add)
				i2next = i2 + 1 // Don't evaluate to this again
				continue        // Keep looking for a matching project
			case +1: // Project has been removed, handled below
				break
			}

			break // Done evaluating this project, move onto the next
		}

		if !matched {
			diff.Remove = append(diff.Remove, pr1)
		}
	}

	// Anything that still hasn't been evaluated are adds
	for i2 := i2next; i2 < len(p2); i2++ {
		lp2 := p2[i2]
		add := buildAddProject(lp2)
		diff.Add = append(diff.Add, add)
	}

	if diff.HashDiff == nil && len(diff.Add) == 0 && len(diff.Remove) == 0 && len(diff.Modify) == 0 {
		return nil // The locks are the equivalent
	}
	return &diff
}

func buildAddProject(lp gps.LockedProject) LockedProjectDiff {
	r2, b2, v2 := getVersionInfo(lp.Version())
	var rev, version, branch *StringDiff
	if r2 != "" {
		rev = &StringDiff{Previous: r2, Current: r2}
	}
	if b2 != "" {
		branch = &StringDiff{Previous: b2, Current: b2}
	}
	if v2 != "" {
		version = &StringDiff{Previous: v2, Current: v2}
	}
	add := LockedProjectDiff{
		Name:     lp.Ident().ProjectRoot,
		Revision: rev,
		Version:  version,
		Branch:   branch,
		Packages: make([]StringDiff, len(lp.Packages())),
	}
	for i, pkg := range lp.Packages() {
		add.Packages[i] = StringDiff{Previous: pkg, Current: pkg}
	}
	return add
}

// diffProjects compares two projects and identifies the differences between them.
// Returns nil if there are no differences
// TODO(carolynvs) this should be moved to gps and updated once the gps unexported fields are available to use.
func diffProjects(lp1 gps.LockedProject, lp2 gps.LockedProject) *LockedProjectDiff {
	diff := LockedProjectDiff{Name: lp1.Ident().ProjectRoot}

	s1 := lp1.Ident().Source
	s2 := lp2.Ident().Source
	if s1 != s2 {
		diff.Source = &StringDiff{Previous: s1, Current: s2}
	}

	r1, b1, v1 := getVersionInfo(lp1.Version())
	r2, b2, v2 := getVersionInfo(lp2.Version())
	if r1 != r2 {
		diff.Revision = &StringDiff{Previous: r1, Current: r2}
	}
	if b1 != b2 {
		diff.Branch = &StringDiff{Previous: b1, Current: b2}
	}
	if v1 != v2 {
		diff.Version = &StringDiff{Previous: v1, Current: v2}
	}

	p1 := lp1.Packages()
	p2 := lp2.Packages()
	if !sort.StringsAreSorted(p1) {
		p1 = make([]string, len(p1))
		copy(p1, lp1.Packages())
		sort.Strings(p1)
	}
	if !sort.StringsAreSorted(p2) {
		p2 = make([]string, len(p2))
		copy(p2, lp2.Packages())
		sort.Strings(p2)
	}

	var i2next int
	for i1 := 0; i1 < len(p1); i1++ {
		pkg1 := p1[i1]

		var matched bool
		for i2 := i2next; i2 < len(p2); i2++ {
			pkg2 := p2[i2]

			switch strings.Compare(pkg1, pkg2) {
			case 0: // Found matching package
				matched = true
				i2next = i2 + 1 // Don't evaluate to this again
			case +1: // Found a new package
				add := StringDiff{Current: pkg2}
				diff.Packages = append(diff.Packages, add)
				i2next = i2 + 1 // Don't evaluate to this again
				continue        // Keep looking for a match
			case -1: // Package has been removed (handled below)
			}

			break // Done evaluating this package, move onto the next
		}

		if !matched {
			diff.Packages = append(diff.Packages, StringDiff{Previous: pkg1})
		}
	}

	// Anything that still hasn't been evaluated are adds
	for i2 := i2next; i2 < len(p2); i2++ {
		pkg2 := p2[i2]
		add := StringDiff{Current: pkg2}
		diff.Packages = append(diff.Packages, add)
	}

	if diff.Source == nil && diff.Version == nil && diff.Revision == nil && len(diff.Packages) == 0 {
		return nil // The projects are equivalent
	}
	return &diff
}
