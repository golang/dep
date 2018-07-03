// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package verify

import (
	"bytes"
	"sort"
	"strings"

	"github.com/golang/dep/gps"
)

// sortLockedProjects returns a sorted copy of lps, or itself if already sorted.
func sortLockedProjects(lps []gps.LockedProject) []gps.LockedProject {
	if len(lps) <= 1 || sort.SliceIsSorted(lps, func(i, j int) bool {
		return lps[i].Ident().Less(lps[j].Ident())
	}) {
		return lps
	}

	cp := make([]gps.LockedProject, len(lps))
	copy(cp, lps)

	sort.Slice(cp, func(i, j int) bool {
		return cp[i].Ident().Less(cp[j].Ident())
	})
	return cp
}

type LockDelta struct {
	AddedImportInputs   []string
	RemovedImportInputs []string
	ProjectDeltas       map[gps.ProjectRoot]LockedProjectDelta
}

type LockedProjectDelta struct {
	Name                         gps.ProjectRoot
	ProjectRemoved, ProjectAdded bool
	LockedProjectPartsDelta
}

type LockedProjectPartsDelta struct {
	PackagesAdded, PackagesRemoved  []string
	VersionBefore, VersionAfter     gps.UnpairedVersion
	RevisionBefore, RevisionAfter   gps.Revision
	SourceBefore, SourceAfter       string
	PruneOptsBefore, PruneOptsAfter gps.PruneOptions
	HashChanged, HashVersionChanged bool
}

// DiffLocks2 compares two locks and computes a semantically rich delta between
// them.
func DiffLocks2(l1, l2 gps.Lock) LockDelta {
	// Default nil locks to empty locks, so that we can still generate a diff
	if l1 == nil {
		if l2 == nil {
			return LockDelta{}
		}
		l1 = gps.SimpleLock{}
	}
	if l2 == nil {
		l2 = gps.SimpleLock{}
	}

	p1, p2 := l1.Projects(), l2.Projects()

	p1 = sortLockedProjects(p1)
	p2 = sortLockedProjects(p2)

	diff := LockDelta{
		ProjectDeltas: make(map[gps.ProjectRoot]LockedProjectDelta),
	}

	var i2next int
	for i1 := 0; i1 < len(p1); i1++ {
		lp1 := p1[i1]
		pr1 := lp1.Ident().ProjectRoot

		lpd := LockedProjectDelta{
			Name: pr1,
		}

		for i2 := i2next; i2 < len(p2); i2++ {
			lp2 := p2[i2]
			pr2 := lp2.Ident().ProjectRoot

			switch strings.Compare(string(pr1), string(pr2)) {
			case 0: // Found a matching project
				lpd.LockedProjectPartsDelta = DiffProjects2(lp1, lp2)
				i2next = i2 + 1 // Don't visit this project again
			case +1: // Found a new project
				diff.ProjectDeltas[pr2] = LockedProjectDelta{
					Name:         pr2,
					ProjectAdded: true,
				}
				i2next = i2 + 1 // Don't visit this project again
				continue        // Keep looking for a matching project
			case -1: // Project has been removed, handled below
				lpd.ProjectRemoved = true
			}

			break // Done evaluating this project, move onto the next
		}

		diff.ProjectDeltas[pr1] = lpd
	}

	// Anything that still hasn't been evaluated are adds
	for i2 := i2next; i2 < len(p2); i2++ {
		lp2 := p2[i2]
		pr2 := lp2.Ident().ProjectRoot
		diff.ProjectDeltas[pr2] = LockedProjectDelta{
			Name:         pr2,
			ProjectAdded: true,
		}
	}

	// Only do the import inputs if both of the locks fulfill the interface, AND
	// both have non-empty inputs.
	il1, ok1 := l1.(gps.LockWithImports)
	il2, ok2 := l2.(gps.LockWithImports)

	if ok1 && ok2 && len(il1.InputImports()) > 0 && len(il2.InputImports()) > 0 {
		diff.AddedImportInputs, diff.RemovedImportInputs = findAddedAndRemoved(il1.InputImports(), il2.InputImports())
	}

	return diff
}

func findAddedAndRemoved(l1, l2 []string) (add, remove []string) {
	// Computing package add/removes could probably be optimized to O(n), but
	// it's not critical path for any known case, so not worth the effort right now.
	p1, p2 := make(map[string]bool, len(l1)), make(map[string]bool, len(l2))

	for _, pkg := range l1 {
		p1[pkg] = true
	}
	for _, pkg := range l2 {
		p2[pkg] = true
	}

	for pkg := range p1 {
		if !p2[pkg] {
			remove = append(remove, pkg)
		}
	}
	for pkg := range p2 {
		if !p1[pkg] {
			add = append(add, pkg)
		}
	}

	return add, remove
}

func DiffProjects2(lp1, lp2 gps.LockedProject) LockedProjectPartsDelta {
	ld := LockedProjectPartsDelta{
		SourceBefore: lp1.Ident().Source,
		SourceAfter:  lp2.Ident().Source,
	}

	ld.PackagesAdded, ld.PackagesRemoved = findAddedAndRemoved(lp1.Packages(), lp2.Packages())

	switch v := lp1.Version().(type) {
	case gps.PairedVersion:
		ld.VersionBefore, ld.RevisionBefore = v.Unpair(), v.Revision()
	case gps.Revision:
		ld.RevisionBefore = v
	case gps.UnpairedVersion:
		// This should ideally never happen
		ld.VersionBefore = v
	}

	switch v := lp2.Version().(type) {
	case gps.PairedVersion:
		ld.VersionAfter, ld.RevisionAfter = v.Unpair(), v.Revision()
	case gps.Revision:
		ld.RevisionAfter = v
	case gps.UnpairedVersion:
		// This should ideally never happen
		ld.VersionAfter = v
	}

	vp1, ok1 := lp1.(VerifiableProject)
	vp2, ok2 := lp2.(VerifiableProject)

	if ok1 && ok2 {
		ld.PruneOptsBefore, ld.PruneOptsAfter = vp1.PruneOpts, vp2.PruneOpts

		if vp1.Digest.HashVersion != vp2.Digest.HashVersion {
			ld.HashVersionChanged = true
		}
		if !bytes.Equal(vp1.Digest.Digest, vp2.Digest.Digest) {
			ld.HashChanged = true
		}
	} else if ok1 {
		ld.PruneOptsBefore = vp1.PruneOpts
		ld.HashVersionChanged = true
		ld.HashChanged = true
	} else if ok2 {
		ld.PruneOptsAfter = vp2.PruneOpts
		ld.HashVersionChanged = true
		ld.HashChanged = true
	}

	return ld
}

// DeltaDimension defines a bitset enumerating all of the different dimensions
// along which a Lock, and its constitutent components, can change.
type DeltaDimension uint32

const (
	InputImportsChanged DeltaDimension = 1 << iota
	ProjectAdded
	ProjectRemoved
	SourceChanged
	VersionChanged
	RevisionChanged
	PackagesChanged
	PruneOptsChanged
	HashVersionChanged
	HashChanged
	AnyChanged = (1 << iota) - 1
)

// Changed indicates whether the delta contains a change along the dimensions
// with their corresponding bits set.
//
// This implementation checks the topmost-level Lock properties
func (ld LockDelta) Changed(dims DeltaDimension) bool {
	if dims&InputImportsChanged != 0 && (len(ld.AddedImportInputs) > 0 || len(ld.RemovedImportInputs) > 0) {
		return true
	}

	for _, ld := range ld.ProjectDeltas {
		if ld.Changed(dims & ^InputImportsChanged) {
			return true
		}
	}

	return false
}

func (ld LockDelta) Changes(DeltaDimension) DeltaDimension {
	var dd DeltaDimension
	if len(ld.AddedImportInputs) > 0 || len(ld.RemovedImportInputs) > 0 {
		dd |= InputImportsChanged
	}

	for _, ld := range ld.ProjectDeltas {
		dd |= ld.Changes()
	}

	return dd
}

// Changed indicates whether the delta contains a change along the dimensions
// with their corresponding bits set.
//
// For example, if only the Revision changed, and this method is called with
// SourceChanged | VersionChanged, it will return false; if it is called with
// VersionChanged | RevisionChanged, it will return true.
func (ld LockedProjectDelta) Changed(flags DeltaDimension) bool {
	if flags&ProjectAdded != 0 && ld.WasAdded() {
		return true
	}

	if flags&ProjectRemoved != 0 && ld.WasRemoved() {
		return true
	}

	return ld.LockedProjectPartsDelta.Changed(flags & ^ProjectAdded & ^ProjectRemoved)
}

func (ld LockedProjectDelta) Changes() DeltaDimension {
	var dd DeltaDimension
	if ld.WasAdded() {
		dd |= ProjectAdded
	}

	if ld.WasRemoved() {
		dd |= ProjectRemoved
	}

	return dd | ld.LockedProjectPartsDelta.Changes()
}

func (ld LockedProjectDelta) WasRemoved() bool {
	return ld.ProjectRemoved
}

func (ld LockedProjectDelta) WasAdded() bool {
	return ld.ProjectAdded
}

func (ld LockedProjectPartsDelta) Changed(flags DeltaDimension) bool {
	if flags&SourceChanged != 0 && ld.SourceChanged() {
		return true
	}
	if flags&RevisionChanged != 0 && ld.RevisionChanged() {
		return true
	}
	if flags&PruneOptsChanged != 0 && ld.PruneOptsChanged() {
		return true
	}
	if flags&HashChanged != 0 && ld.HashChanged {
		return true
	}
	if flags&HashVersionChanged != 0 && ld.HashVersionChanged {
		return true
	}
	if flags&VersionChanged != 0 && ld.VersionChanged() {
		return true
	}
	if flags&PackagesChanged != 0 && ld.PackagesChanged() {
		return true
	}

	return false
}

func (ld LockedProjectPartsDelta) Changes() DeltaDimension {
	var dd DeltaDimension
	if ld.SourceChanged() {
		dd |= SourceChanged
	}
	if ld.RevisionChanged() {
		dd |= RevisionChanged
	}
	if ld.PruneOptsChanged() {
		dd |= PruneOptsChanged
	}
	if ld.HashChanged {
		dd |= HashChanged
	}
	if ld.HashVersionChanged {
		dd |= HashVersionChanged
	}
	if ld.VersionChanged() {
		dd |= VersionChanged
	}
	if ld.PackagesChanged() {
		dd |= PackagesChanged
	}

	return dd
}

func (ld LockedProjectPartsDelta) SourceChanged() bool {
	return ld.SourceBefore != ld.SourceAfter
}

func (ld LockedProjectPartsDelta) VersionChanged() bool {
	if ld.VersionBefore == nil && ld.VersionAfter == nil {
		return false
	} else if (ld.VersionBefore == nil || ld.VersionAfter == nil) || (ld.VersionBefore.Type() != ld.VersionAfter.Type()) {
		return true
	} else if !ld.VersionBefore.Matches(ld.VersionAfter) {
		return true
	}

	return false
}

func (ld LockedProjectPartsDelta) VersionTypeChanged() bool {
	if ld.VersionBefore == nil && ld.VersionAfter == nil {
		return false
	} else if (ld.VersionBefore == nil || ld.VersionAfter == nil) || (ld.VersionBefore.Type() != ld.VersionAfter.Type()) {
		return true
	}

	return false
}

func (ld LockedProjectPartsDelta) RevisionChanged() bool {
	return ld.RevisionBefore != ld.RevisionAfter
}

func (ld LockedProjectPartsDelta) PackagesChanged() bool {
	return len(ld.PackagesAdded) > 0 || len(ld.PackagesRemoved) > 0
}

func (ld LockedProjectPartsDelta) PruneOptsChanged() bool {
	return ld.PruneOptsBefore != ld.PruneOptsAfter
}

//type VendorDiff struct {
//LockDelta    LockDelta
//VendorStatus map[string]VendorStatus
//}
