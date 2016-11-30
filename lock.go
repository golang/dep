// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sdboyer/gps"
)

type lock struct {
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

func readLock(r io.Reader) (*lock, error) {
	rl := rawLock{}
	err := json.NewDecoder(r).Decode(&rl)
	if err != nil {
		return nil, err
	}

	b, err := hex.DecodeString(rl.Memo)
	if err != nil {
		return nil, fmt.Errorf("invalid hash digest in lock's memo field")
	}
	l := &lock{
		Memo: b,
		P:    make([]gps.LockedProject, len(rl.P)),
	}

	for i, ld := range rl.P {
		r := gps.Revision(ld.Revision)

		var v gps.Version
		if ld.Version != "" {
			if ld.Branch != "" {
				return nil, fmt.Errorf("lock file specified both a branch (%s) and version (%s) for %s", ld.Branch, ld.Version, ld.Name)
			}
			v = gps.NewVersion(ld.Version).Is(r)
		} else if ld.Branch != "" {
			v = gps.NewBranch(ld.Branch).Is(r)
		} else if r == "" {
			return nil, fmt.Errorf("lock file has entry for %s, but specifies no version", ld.Name)
		} else {
			v = r
		}

		id := gps.ProjectIdentifier{
			ProjectRoot: gps.ProjectRoot(ld.Name),
			NetworkName: ld.Repository,
		}
		l.P[i] = gps.NewLockedProject(id, v, ld.Packages)
	}

	return l, nil
}

func (l *lock) InputHash() []byte {
	return l.Memo
}

func (l *lock) Projects() []gps.LockedProject {
	return l.P
}

func (l *lock) MarshalJSON() ([]byte, error) {
	raw := rawLock{
		Memo: hex.EncodeToString(l.Memo),
		P: make([]lockedDep, len(l.P))
	}

	for k, lp := range l.P {
		id := lp.Ident()
		ld := lockedDep{
			Name:       string(id.ProjectRoot()),
			Repository: id.NetworkName,
			Packages:   lp.Packages(),
		}

		switch tv := lp.Version().(type) {
		case gps.UnpairedVersion:
			ld.Version = tv
		case gps.Revision:
			ld.Revision = tv
		case gps.PairedVersion:
			ld.Version = tv.Unpair()
			ld.Revision = tv.Underlying()
		}

		raw.P[k] = ld
	}

	return json.Marshal(raw)
}
