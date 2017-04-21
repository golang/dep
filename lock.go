// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"encoding/hex"
	"io"
	"sort"

	"bytes"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	"github.com/golang/dep/gps"
)

const LockName = "Gopkg.lock"

type Lock struct {
	Memo []byte
	P    []gps.LockedProject
}

type rawLock struct {
	Memo     string             `toml:"memo"`
	Projects []rawLockedProject `toml:"projects"`
}

type rawLockedProject struct {
	Name     string   `toml:"name"`
	Branch   string   `toml:"branch,omitempty"`
	Revision string   `toml:"revision"`
	Version  string   `toml:"version,omitempty"`
	Source   string   `toml:"source,omitempty"`
	Packages []string `toml:"packages"`
}

func readLock(r io.Reader) (*Lock, error) {
	buf := &bytes.Buffer{}
	_, err := buf.ReadFrom(r)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read byte stream")
	}

	raw := rawLock{}
	err = toml.Unmarshal(buf.Bytes(), &raw)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse the lock as TOML")
	}

	return fromRawLock(raw)
}

func fromRawLock(raw rawLock) (*Lock, error) {
	var err error
	l := &Lock{
		P: make([]gps.LockedProject, len(raw.Projects)),
	}

	l.Memo, err = hex.DecodeString(raw.Memo)
	if err != nil {
		return nil, errors.Errorf("invalid hash digest in lock's memo field")
	}

	for i, ld := range raw.Projects {
		r := gps.Revision(ld.Revision)

		var v gps.Version = r
		if ld.Version != "" {
			if ld.Branch != "" {
				return nil, errors.Errorf("lock file specified both a branch (%s) and version (%s) for %s", ld.Branch, ld.Version, ld.Name)
			}
			v = gps.NewVersion(ld.Version).Is(r)
		} else if ld.Branch != "" {
			v = gps.NewBranch(ld.Branch).Is(r)
		} else if r == "" {
			return nil, errors.Errorf("lock file has entry for %s, but specifies no branch or version", ld.Name)
		}

		id := gps.ProjectIdentifier{
			ProjectRoot: gps.ProjectRoot(ld.Name),
			Source:      ld.Source,
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

// toRaw converts the manifest into a representation suitable to write to the lock file
func (l *Lock) toRaw() rawLock {
	raw := rawLock{
		Memo:     hex.EncodeToString(l.Memo),
		Projects: make([]rawLockedProject, len(l.P)),
	}

	sort.Sort(SortedLockedProjects(l.P))

	for k, lp := range l.P {
		id := lp.Ident()
		ld := rawLockedProject{
			Name:     string(id.ProjectRoot),
			Source:   id.Source,
			Packages: lp.Packages(),
		}

		v := lp.Version()
		ld.Revision, ld.Branch, ld.Version = getVersionInfo(v)

		raw.Projects[k] = ld
	}

	// TODO sort output - #15

	return raw
}

func (l *Lock) MarshalTOML() ([]byte, error) {
	raw := l.toRaw()
	result, err := toml.Marshal(raw)
	return result, errors.Wrap(err, "Unable to marshal lock to TOML string")
}

// TODO(carolynvs) this should be moved to gps
func getVersionInfo(v gps.Version) (revision string, branch string, version string) {
	// Figure out how to get the underlying revision
	switch tv := v.(type) {
	case gps.UnpairedVersion:
	// TODO we could error here, if we want to be very defensive about not
	// allowing a lock to be written if without an immmutable revision
	case gps.Revision:
		revision = tv.String()
	case gps.PairedVersion:
		revision = tv.Underlying().String()
	}

	switch v.Type() {
	case gps.IsBranch:
		branch = v.String()
	case gps.IsSemver, gps.IsVersion:
		version = v.String()
	}

	return
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
