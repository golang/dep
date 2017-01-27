// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/sdboyer/gps"
)

const LockName = "lock.json"

type Lock struct {
	Memo []byte
	P    []gps.LockedProject
}

type rawLock struct {
	Memo string      `json:"memo"`
	P    []lockedDep `json:"projects"`
}

type lockedDep struct {
	Name       string   `json:"name"`
	Version    string   `json:"version,omitempty"`
	Branch     string   `json:"branch,omitempty"`
	Revision   string   `json:"revision"`
	Repository string   `json:"repo,omitempty"`
	Packages   []string `json:"packages"`
}

func readLock(r io.Reader) (*Lock, error) {
	rl := rawLock{}
	err := json.NewDecoder(r).Decode(&rl)
	if err != nil {
		return nil, err
	}

	b, err := hex.DecodeString(rl.Memo)
	if err != nil {
		return nil, fmt.Errorf("invalid hash digest in lock's memo field")
	}
	l := &Lock{
		Memo: b,
		P:    make([]gps.LockedProject, len(rl.P)),
	}

	for i, ld := range rl.P {
		r := gps.Revision(ld.Revision)

		var v gps.Version = r
		if ld.Version != "" {
			if ld.Branch != "" {
				return nil, fmt.Errorf("lock file specified both a branch (%s) and version (%s) for %s", ld.Branch, ld.Version, ld.Name)
			}
			v = gps.NewVersion(ld.Version).Is(r)
		} else if ld.Branch != "" {
			v = gps.NewBranch(ld.Branch).Is(r)
		} else if r == "" {
			return nil, fmt.Errorf("lock file has entry for %s, but specifies no version", ld.Name)
		}

		id := gps.ProjectIdentifier{
			ProjectRoot: gps.ProjectRoot(ld.Name),
			Source:      ld.Repository,
		}
		l.P[i] = gps.NewLockedProject(id, v, ld.Packages)
	}

	return l, nil
}

func (l *Lock) InputHash() []byte {
	return l.Memo
}

func (l *Lock) Projects() []gps.LockedProject {
	return l.P
}

func (l *Lock) MarshalJSON() ([]byte, error) {
	raw := rawLock{
		Memo: hex.EncodeToString(l.Memo),
		P:    make([]lockedDep, len(l.P)),
	}

	sort.Sort(SortedLockedProjects(l.P))

	for k, lp := range l.P {
		id := lp.Ident()
		ld := lockedDep{
			Name:       string(id.ProjectRoot),
			Repository: id.Source,
			Packages:   lp.Packages(),
		}

		v := lp.Version()
		// Figure out how to get the underlying revision
		switch tv := v.(type) {
		case gps.UnpairedVersion:
			// TODO we could error here, if we want to be very defensive about not
			// allowing a lock to be written if without an immmutable revision
		case gps.Revision:
			ld.Revision = tv.String()
		case gps.PairedVersion:
			ld.Revision = tv.Underlying().String()
		}

		switch v.Type() {
		case gps.IsBranch:
			ld.Branch = v.String()
		case gps.IsSemver, gps.IsVersion:
			ld.Version = v.String()
		}

		raw.P[k] = ld
	}

	// TODO sort output - #15

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "    ")
	enc.SetEscapeHTML(false)
	err := enc.Encode(raw)

	return buf.Bytes(), err
}

// LockFromInterface converts an arbitrary gps.Lock to dep's representation of a
// lock. If the input is already dep's *lock, the input is returned directly.
//
// Data is defensively copied wherever necessary to ensure the resulting *lock
// shares no memory with the original lock.
//
// As gps.Solution is a superset of gps.Lock, this can also be used to convert
// solutions to dep's lock format.
func LockFromInterface(in gps.Lock) *Lock {
	if in == nil {
		return nil
	} else if l, ok := in.(*Lock); ok {
		return l
	}

	h, p := in.InputHash(), in.Projects()

	l := &Lock{
		Memo: make([]byte, len(h)),
		P:    make([]gps.LockedProject, len(p)),
	}

	copy(l.Memo, h)
	copy(l.P, p)
	return l
}

type SortedLockedProjects []gps.LockedProject

func (s SortedLockedProjects) Len() int      { return len(s) }
func (s SortedLockedProjects) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s SortedLockedProjects) Less(i, j int) bool {
	l, r := s[i].Ident(), s[j].Ident()

	if l.ProjectRoot < r.ProjectRoot {
		return true
	}
	if r.ProjectRoot < l.ProjectRoot {
		return false
	}

	return l.Source < r.Source
}

// locksAreEquivalent compares two locks to see if they differ. If EITHER lock
// is nil, or their memos do not match, or any projects differ, then false is
// returned.
func locksAreEquivalent(l, r *Lock) bool {
	if l == nil || r == nil {
		return false
	}

	if !bytes.Equal(l.Memo, r.Memo) {
		return false
	}

	if len(l.P) != len(r.P) {
		return false
	}

	sort.Sort(SortedLockedProjects(l.P))
	sort.Sort(SortedLockedProjects(r.P))

	for k, lp := range l.P {
		// TODO(sdboyer) gps will be adding a func to compare LockedProjects in the next
		// version; we can swap that in here when it lands
		rp := r.P[k]
		if !lp.Eq(rp) {
			return false
		}
	}

	return true
}
