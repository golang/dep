// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sdboyer/gps"
)

type manifest struct {
	Dependencies gps.ProjectConstraints
	Ovr          gps.ProjectConstraints
	Ignores      []string
}

type rawManifest struct {
	Dependencies map[string]possibleProps `json:"dependencies,omitempty"`
	Overrides    map[string]possibleProps `json:"overrides,omitempty"`
	Ignores      []string                 `json:"ignores,omitempty"`
}

type possibleProps struct {
	Branch      string `json:"branch,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Version     string `json:"version,omitempty"`
	NetworkName string `json:"source,omitempty"`
}

func newRawManifest() rawManifest {
	return rawManifest{
		Dependencies: make(map[string]possibleProps),
		Overrides:    make(map[string]possibleProps),
		Ignores:      make([]string, 0),
	}
}

func readManifest(r io.Reader) (*manifest, error) {
	rm := rawManifest{}
	err := json.NewDecoder(r).Decode(&rm)
	if err != nil {
		return nil, err
	}

	m := &manifest{
		Dependencies: make(gps.ProjectConstraints, len(rm.Dependencies)),
		Ovr:          make(gps.ProjectConstraints, len(rm.Overrides)),
		Ignores:      rm.Ignores,
	}

	for n, pp := range rm.Dependencies {
		m.Dependencies[gps.ProjectRoot(n)], err = toProps(n, pp)
		if err != nil {
			return nil, err
		}
	}

	for n, pp := range rm.Overrides {
		m.Ovr[gps.ProjectRoot(n)], err = toProps(n, pp)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// toProps interprets the string representations of project information held in
// a possibleProps, converting them into a proper gps.ProjectProperties. An
// error is returned if the possibleProps contains some invalid combination -
// for example, if both a branch and version constraint are specified.
func toProps(n string, p possibleProps) (pp gps.ProjectProperties, err error) {
	if p.Branch != "" {
		if p.Version != "" || p.Revision != "" {
			return pp, fmt.Errorf("multiple constraints specified for %s, can only specify one", n)
		}
		pp.Constraint = gps.NewBranch(p.Branch)
	} else if p.Version != "" {
		if p.Revision != "" {
			return pp, fmt.Errorf("multiple constraints specified for %s, can only specify one", n)
		}

		// always semver if we can
		pp.Constraint, err = gps.NewSemverConstraint(p.Version)
		if err != nil {
			// but if not, fall back on plain versions
			pp.Constraint = gps.NewVersion(p.Version)
		}
	} else if p.Revision != "" {
		pp.Constraint = gps.Revision(p.Revision)
	} else {
		// If the user specifies nothing, it means an open constraint (accept
		// anything).
		pp.Constraint = gps.Any()
	}

	pp.NetworkName = p.NetworkName
	return pp, nil
}

func (m *manifest) MarshalJSON() ([]byte, error) {
	raw := rawManifest{
		Dependencies: make(map[string]possibleProps, len(m.Dependencies)),
		Overrides:    make(map[string]possibleProps, len(m.Ovr)),
		Ignores:      m.Ignores,
	}

	for n, pp := range m.Dependencies {
		raw.Dependencies[string(n)] = toPossible(pp)
	}

	for n, pp := range m.Ovr {
		raw.Overrides[string(n)] = toPossible(pp)
	}

	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	// Semver range ops, > and <, get turned into unicode code points. This is a
	// nice example of why using JSON for files like this is not the best
	b = bytes.Replace(b, []byte("\\u003c"), []byte("<"), -1)
	b = bytes.Replace(b, []byte("\\u003e"), []byte(">"), -1)
	return b, nil
}

func toPossible(pp gps.ProjectProperties) (p possibleProps) {
	p.NetworkName = pp.NetworkName

	if v, ok := pp.Constraint.(gps.Version); ok {
		switch v.Type() {
		case "revision":
			p.Revision = v.String()
		case "branch":
			p.Branch = v.String()
		case "semver", "version":
			p.Version = v.String()
		}
	} else {
		// We simply don't allow for a case where the user could directly
		// express a 'none' constraint, so we can ignore it here. We also ignore
		// the 'any' case, because that's the other possibility, and it's what
		// we interpret not having any constraint expressions at all to mean.
		//if !gps.IsAny(pp.Constraint) && !gps.IsNone(pp.Constraint) {
		if !gps.IsAny(pp.Constraint) {
			// Has to be a semver range.
			p.Version = pp.Constraint.String()
		}
	}

	return
}

func (m *manifest) DependencyConstraints() gps.ProjectConstraints {
	return m.Dependencies
}

func (m *manifest) TestDependencyConstraints() gps.ProjectConstraints {
	// TODO decide whether we're going to incorporate this or not
	return nil
}

func (m *manifest) Overrides() gps.ProjectConstraints {
	return m.Ovr
}

func (m *manifest) IgnorePackages() map[string]bool {
	if len(m.Ignores) == 0 {
		return nil
	}

	mp := make(map[string]bool, len(m.Ignores))
	for _, i := range m.Ignores {
		mp[i] = true
	}

	return mp
}
