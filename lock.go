// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"

	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
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
	Name     string   `json:"name"`
	Version  string   `json:"version,omitempty"`
	Branch   string   `json:"branch,omitempty"`
	Revision string   `json:"revision"`
	Source   string   `json:"source,omitempty"`
	Packages []string `json:"packages"`
}

func readLock(r io.Reader) (*Lock, error) {
	rl := rawLock{}
	err := json.NewDecoder(r).Decode(&rl)
	if err != nil {
		return nil, err
	}

	b, err := hex.DecodeString(rl.Memo)
	if err != nil {
		return nil, errors.Errorf("invalid hash digest in lock's memo field")
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
		Memo: hex.EncodeToString(l.Memo),
		P:    make([]lockedDep, len(l.P)),
	}

	sort.Sort(SortedLockedProjects(l.P))

	for k, lp := range l.P {
		id := lp.Ident()
		ld := lockedDep{
			Name:     string(id.ProjectRoot),
			Source:   id.Source,
			Packages: lp.Packages(),
		}

		v := lp.Version()
		ld.Revision, ld.Branch, ld.Version = getVersionInfo(v)

		raw.P[k] = ld
	}

	// TODO sort output - #15

	return raw
}

func (l *Lock) MarshalJSON() ([]byte, error) {
	raw := l.toRaw()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "    ")
	enc.SetEscapeHTML(false)
	err := enc.Encode(raw)

	return buf.Bytes(), err
}

func (l *Lock) MarshalTOML() (string, error) {
	raw := l.toRaw()

	// TODO(carolynvs) Consider adding reflection-based marshal functionality to go-toml
	m := make(map[string]interface{})
	m["memo"] = raw.Memo
	p := make([]map[string]interface{}, len(raw.P))
	for i := 0; i < len(p); i++ {
		prj := make(map[string]interface{})
		prj["name"] = raw.P[i].Name
		prj["revision"] = raw.P[i].Revision

		if raw.P[i].Source != "" {
			prj["source"] = raw.P[i].Source
		}
		if raw.P[i].Branch != "" {
			prj["branch"] = raw.P[i].Branch
		}
		if raw.P[i].Version != "" {
			prj["version"] = raw.P[i].Version
		}

		pkgs := make([]interface{}, len(raw.P[i].Packages))
		for j := range raw.P[i].Packages {
			pkgs[j] = raw.P[i].Packages[j]
		}
		prj["packages"] = pkgs

		p[i] = prj
	}
	m["projects"] = p

	t, err := toml.TreeFromMap(m)
	if err != nil {
		return "", errors.Wrap(err, "Unable to marshal lock to TOML tree")
	}

	result, err := t.ToTomlString()
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
