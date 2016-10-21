// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
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
	Dependencies map[string]possibleProps `json:"dependencies"`
	Overrides    map[string]possibleProps `json:"overrides"`
	Ignores      []string                 `json:"ignores"`
}

type possibleProps struct {
	Branch      string `json:"branch"`
	Revision    string `json:"revision"`
	Version     string `json:"version"`
	NetworkName string `json:"network_name"`
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
