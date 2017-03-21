// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"encoding/json"
	"io"

	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

const ManifestName = "manifest.toml"

type Manifest struct {
	Dependencies gps.ProjectConstraints
	Ovr          gps.ProjectConstraints
	Ignores      []string
	Required     []string
}

type rawManifest struct {
	Dependencies map[string]possibleProps
	Overrides    map[string]possibleProps
	Ignores      []string
	Required     []string
}

func newRawManifest() rawManifest {
	return rawManifest{
		Dependencies: make(map[string]possibleProps),
		Overrides:    make(map[string]possibleProps),
		Ignores:      make([]string, 0),
		Required:     make([]string, 0),
	}
}

type possibleProps struct {
	Branch   string
	Revision string
	Version  string
	Source   string
}

func readManifest(r io.Reader) (*Manifest, error) {
	rm := rawManifest{}
	err := json.NewDecoder(r).Decode(&rm)
	if err != nil {
		return nil, err
	}
	m := &Manifest{
		Dependencies: make(gps.ProjectConstraints, len(rm.Dependencies)),
		Ovr:          make(gps.ProjectConstraints, len(rm.Overrides)),
		Ignores:      rm.Ignores,
		Required:     rm.Required,
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
			return pp, errors.Errorf("multiple constraints specified for %s, can only specify one", n)
		}
		pp.Constraint = gps.NewBranch(p.Branch)
	} else if p.Version != "" {
		if p.Revision != "" {
			return pp, errors.Errorf("multiple constraints specified for %s, can only specify one", n)
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

	pp.Source = p.Source
	return pp, nil
}

// toRaw converts the manifest into a representation suitable to write to the manifest file
func (m *Manifest) toRaw() rawManifest {
	raw := rawManifest{
		Dependencies: make(map[string]possibleProps, len(m.Dependencies)),
		Overrides:    make(map[string]possibleProps, len(m.Ovr)),
		Ignores:      m.Ignores,
		Required:     m.Required,
	}
	for n, pp := range m.Dependencies {
		raw.Dependencies[string(n)] = toPossible(pp)
	}
	for n, pp := range m.Ovr {
		raw.Overrides[string(n)] = toPossible(pp)
	}
	return raw
}

func (m *Manifest) MarshalTOML() (string, error) {
	raw := m.toRaw()

	// TODO(carolynvs) Consider adding reflection-based marshal functionality to go-toml
	copyProjects := func(src map[string]possibleProps) []map[string]interface{} {
		dest := make([]map[string]interface{}, 0, len(src))
		for prjName, srcPrj := range src {
			prj := make(map[string]interface{})
			prj["name"] = prjName
			if srcPrj.Source != "" {
				prj["source"] = srcPrj.Source
			}
			if srcPrj.Branch != "" {
				prj["branch"] = srcPrj.Branch
			}
			if srcPrj.Version != "" {
				prj["version"] = srcPrj.Version
			}
			if srcPrj.Revision != "" {
				prj["revision"] = srcPrj.Revision
			}
			dest = append(dest, prj)

		}
		return dest
	}

	copyProjectRefs := func(src []string) []interface{} {
		dest := make([]interface{}, len(src))
		for i := range src {
			dest[i] = src[i]
		}
		return dest
	}

	data := make(map[string]interface{})
	if len(raw.Dependencies) > 0 {
		data["dependencies"] = copyProjects(raw.Dependencies)
	}
	if len(raw.Overrides) > 0 {
		data["overrides"] = copyProjects(raw.Overrides)
	}
	if len(raw.Ignores) > 0 {
		data["ignores"] = copyProjectRefs(raw.Ignores)
	}
	if len(raw.Required) > 0 {
		data["required"] = copyProjectRefs(raw.Required)
	}

	tree, err := toml.TreeFromMap(data)
	if err != nil {
		return "", errors.Wrap(err, "Unable to marshal the lock to a TOML tree")
	}
	result, err := tree.ToTomlString()
	return result, errors.Wrap(err, "Unable to marshal the lock to a TOML string")
}

func toPossible(pp gps.ProjectProperties) possibleProps {
	p := possibleProps{
		Source: pp.Source,
	}

	if v, ok := pp.Constraint.(gps.Version); ok {
		switch v.Type() {
		case gps.IsRevision:
			p.Revision = v.String()
		case gps.IsBranch:
			p.Branch = v.String()
		case gps.IsSemver, gps.IsVersion:
			p.Version = v.String()
		}
		return p
	}

	// We simply don't allow for a case where the user could directly
	// express a 'none' constraint, so we can ignore it here. We also ignore
	// the 'any' case, because that's the other possibility, and it's what
	// we interpret not having any constraint expressions at all to mean.
	// if !gps.IsAny(pp.Constraint) && !gps.IsNone(pp.Constraint) {
	if !gps.IsAny(pp.Constraint) && pp.Constraint != nil {
		// Has to be a semver range.
		p.Version = pp.Constraint.String()
	}
	return p
}

func (m *Manifest) DependencyConstraints() gps.ProjectConstraints {
	return m.Dependencies
}

func (m *Manifest) TestDependencyConstraints() gps.ProjectConstraints {
	// TODO decide whether we're going to incorporate this or not
	return nil
}

func (m *Manifest) Overrides() gps.ProjectConstraints {
	return m.Ovr
}

func (m *Manifest) IgnoredPackages() map[string]bool {
	if len(m.Ignores) == 0 {
		return nil
	}

	mp := make(map[string]bool, len(m.Ignores))
	for _, i := range m.Ignores {
		mp[i] = true
	}

	return mp
}

func (m *Manifest) RequiredPackages() map[string]bool {
	if len(m.Required) == 0 {
		return nil
	}

	mp := make(map[string]bool, len(m.Required))
	for _, i := range m.Required {
		mp[i] = true
	}

	return mp
}
