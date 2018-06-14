// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"io"
	"sort"

	"github.com/golang/dep/gps"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
)

// LockName is the lock file name used by dep.
const LockName = "Gopkg.lock"

// Lock holds lock file data and implements gps.Lock.
type Lock struct {
	SolveMeta SolveMeta
	P         []gps.LockedProject
}

// SolveMeta holds solver meta data.
type SolveMeta struct {
	AnalyzerName    string
	AnalyzerVersion int
	SolverName      string
	SolverVersion   int
}

type rawLock struct {
	SolveMeta solveMeta          `toml:"solve-meta"`
	Projects  []rawLockedProject `toml:"projects"`
}

type solveMeta struct {
	AnalyzerName    string   `toml:"analyzer-name"`
	AnalyzerVersion int      `toml:"analyzer-version"`
	SolverName      string   `toml:"solver-name"`
	SolverVersion   int      `toml:"solver-version"`
	InputImports    []string `toml:"input-imports"`
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

	l.SolveMeta.AnalyzerName = raw.SolveMeta.AnalyzerName
	l.SolveMeta.AnalyzerVersion = raw.SolveMeta.AnalyzerVersion
	l.SolveMeta.SolverName = raw.SolveMeta.SolverName
	l.SolveMeta.SolverVersion = raw.SolveMeta.SolverVersion
	l.SolveMeta.InputImports = raw.SolveMeta.InputImports

	for i, ld := range raw.Projects {
		r := gps.Revision(ld.Revision)

		var v gps.Version = r
		if ld.Version != "" {
			if ld.Branch != "" {
				return nil, errors.Errorf("lock file specified both a branch (%s) and version (%s) for %s", ld.Branch, ld.Version, ld.Name)
			}
			v = gps.NewVersion(ld.Version).Pair(r)
		} else if ld.Branch != "" {
			v = gps.NewBranch(ld.Branch).Pair(r)
		} else if r == "" {
			return nil, errors.Errorf("lock file has entry for %s, but specifies no branch or version", ld.Name)
		}

		id := gps.ProjectIdentifier{
			ProjectRoot: gps.ProjectRoot(ld.Name),
			Source:      ld.Source,
		}
		l.P[i] = gps.NewLockedProject(id, v, ld.Packages, ld.Imports)
	}

	return l, nil
}

// InputsDigest returns the hash of inputs which produced this lock data.
//
// TODO(sdboyer) remove, this is now deprecated
func (l *Lock) InputsDigest() []byte {
	return nil
}

// Projects returns the list of LockedProjects contained in the lock data.
func (l *Lock) Projects() []gps.LockedProject {
	return l.P
}

// HasProjectWithRoot checks if the lock contains a project with the provided
// ProjectRoot.
//
// This check is O(n) in the number of projects.
func (l *Lock) HasProjectWithRoot(root gps.ProjectRoot) bool {
	for _, p := range l.P {
		if p.Ident().ProjectRoot == root {
			return true
		}
	}

	return false
}

// toRaw converts the manifest into a representation suitable to write to the lock file
func (l *Lock) toRaw() rawLock {
	raw := rawLock{
		SolveMeta: solveMeta{
			AnalyzerName:    l.SolveMeta.AnalyzerName,
			AnalyzerVersion: l.SolveMeta.AnalyzerVersion,
			SolverName:      l.SolveMeta.SolverName,
			SolverVersion:   l.SolveMeta.SolverVersion,
		},
		Projects: make([]rawLockedProject, len(l.P)),
	}

	sort.Slice(l.P, func(i, j int) bool {
		return l.P[i].Ident().Less(l.P[j].Ident())
	})

	for k, lp := range l.P {
		id := lp.Ident()
		ld := rawLockedProject{
			Name:     string(id.ProjectRoot),
			Source:   id.Source,
			Packages: lp.Packages(),
		}

		v := lp.Version()
		ld.Revision, ld.Branch, ld.Version = gps.VersionComponentStrings(v)

		raw.Projects[k] = ld
	}

	return raw
}

// MarshalTOML serializes this lock into TOML via an intermediate raw form.
func (l *Lock) MarshalTOML() ([]byte, error) {
	raw := l.toRaw()
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf).ArraysWithOneElementPerLine(true)
	err := enc.Encode(raw)
	return buf.Bytes(), errors.Wrap(err, "Unable to marshal lock to TOML string")
}

// LockFromSolution converts a gps.Solution to dep's representation of a lock.
//
// Data is defensively copied wherever necessary to ensure the resulting *lock
// shares no memory with the input solution.
func LockFromSolution(in gps.Solution) *Lock {
	p := in.Projects()

	l := &Lock{
		SolveMeta: SolveMeta{
			AnalyzerName:    in.AnalyzerName(),
			AnalyzerVersion: in.AnalyzerVersion(),
			SolverName:      in.SolverName(),
			SolverVersion:   in.SolverVersion(),
		},
		P: make([]gps.LockedProject, len(p)),
	}

	copy(l.P, p)
	return l
}
