// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"io"
	"sort"

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
	Dependencies []rawProject
	Overrides    []rawProject
	Ignores      []string
	Required     []string
}

type rawProject struct {
	Name     string
	Branch   string
	Revision string
	Version  string
	Source   string
}

func readManifest(r io.Reader) (*Manifest, error) {
	tree, err := toml.LoadReader(r)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse the manifest as TOML")
	}

	mapper := &tomlMapper{Tree: tree}
	raw := rawManifest{
		Dependencies: readTableAsProjects(mapper, "dependencies"),
		Overrides:    readTableAsProjects(mapper, "overrides"),
		Required:     readKeyAsStringList(mapper, "required"),
		Ignores:      readKeyAsStringList(mapper, "ignores"),
	}

	if mapper.Error != nil {
		return nil, errors.Wrap(mapper.Error, "Invalid manifest structure")
	}
	return fromRawManifest(raw)
}

func fromRawManifest(raw rawManifest) (*Manifest, error) {
	m := &Manifest{
		Dependencies: make(gps.ProjectConstraints, len(raw.Dependencies)),
		Ovr:          make(gps.ProjectConstraints, len(raw.Overrides)),
		Ignores:      raw.Ignores,
		Required:     raw.Required,
	}

	for i := 0; i < len(raw.Dependencies); i++ {
		name, prj, err := toProject(raw.Dependencies[i])
		if err != nil {
			return nil, err
		}
		m.Dependencies[name] = prj
	}

	for i := 0; i < len(raw.Overrides); i++ {
		name, prj, err := toProject(raw.Overrides[i])
		if err != nil {
			return nil, err
		}
		m.Ovr[name] = prj
	}

	return m, nil
}

// toProject interprets the string representations of project information held in
// a rawProject, converting them into a proper gps.ProjectProperties. An
// error is returned if the rawProject contains some invalid combination -
// for example, if both a branch and version constraint are specified.
func toProject(raw rawProject) (n gps.ProjectRoot, pp gps.ProjectProperties, err error) {
	n = gps.ProjectRoot(raw.Name)
	if raw.Branch != "" {
		if raw.Version != "" || raw.Revision != "" {
			return n, pp, errors.Errorf("multiple constraints specified for %s, can only specify one", n)
		}
		pp.Constraint = gps.NewBranch(raw.Branch)
	} else if raw.Version != "" {
		if raw.Revision != "" {
			return n, pp, errors.Errorf("multiple constraints specified for %s, can only specify one", n)
		}

		// always semver if we can
		pp.Constraint, err = gps.NewSemverConstraint(raw.Version)
		if err != nil {
			// but if not, fall back on plain versions
			pp.Constraint = gps.NewVersion(raw.Version)
		}
	} else if raw.Revision != "" {
		pp.Constraint = gps.Revision(raw.Revision)
	} else {
		// If the user specifies nothing, it means an open constraint (accept
		// anything).
		pp.Constraint = gps.Any()
	}

	pp.Source = raw.Source
	return n, pp, nil
}

// toRaw converts the manifest into a representation suitable to write to the manifest file
func (m *Manifest) toRaw() rawManifest {
	raw := rawManifest{
		Dependencies: make([]rawProject, 0, len(m.Dependencies)),
		Overrides:    make([]rawProject, 0, len(m.Ovr)),
		Ignores:      m.Ignores,
		Required:     m.Required,
	}
	for n, prj := range m.Dependencies {
		raw.Dependencies = append(raw.Dependencies, toRawProject(n, prj))
	}
	sort.Sort(sortedRawProjects(raw.Dependencies))

	for n, prj := range m.Ovr {
		raw.Overrides = append(raw.Overrides, toRawProject(n, prj))
	}
	sort.Sort(sortedRawProjects(raw.Overrides))

	return raw
}

// TODO(carolynvs) when gps is moved, we can use the unexported gps.sortedConstraints
type sortedRawProjects []rawProject

func (s sortedRawProjects) Len() int      { return len(s) }
func (s sortedRawProjects) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortedRawProjects) Less(i, j int) bool {
	l, r := s[i], s[j]

	if l.Name < r.Name {
		return true
	}
	if r.Name < l.Name {
		return false
	}

	return l.Source < r.Source
}

func (m *Manifest) MarshalTOML() (string, error) {
	raw := m.toRaw()

	mapRawProject := func(raw rawProject) map[string]interface{} {
		prj := make(map[string]interface{})
		prj["name"] = raw.Name
		if raw.Source != "" {
			prj["source"] = raw.Source
		}
		if raw.Branch != "" {
			prj["branch"] = raw.Branch
		}
		if raw.Version != "" {
			prj["version"] = raw.Version
		}
		if raw.Revision != "" {
			prj["revision"] = raw.Revision
		}
		return prj
	}

	mapRawProjects := func(src []rawProject) []map[string]interface{} {
		dest := make([]map[string]interface{}, len(src))
		for i := 0; i < len(src); i++ {
			dest[i] = mapRawProject(src[i])
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
		data["dependencies"] = mapRawProjects(raw.Dependencies)
	}
	if len(raw.Overrides) > 0 {
		data["overrides"] = mapRawProjects(raw.Overrides)
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

func toRawProject(name gps.ProjectRoot, project gps.ProjectProperties) rawProject {
	raw := rawProject{
		Name:   string(name),
		Source: project.Source,
	}

	if v, ok := project.Constraint.(gps.Version); ok {
		switch v.Type() {
		case gps.IsRevision:
			raw.Revision = v.String()
		case gps.IsBranch:
			raw.Branch = v.String()
		case gps.IsSemver, gps.IsVersion:
			raw.Version = v.String()
		}
		return raw
	}

	// We simply don't allow for a case where the user could directly
	// express a 'none' constraint, so we can ignore it here. We also ignore
	// the 'any' case, because that's the other possibility, and it's what
	// we interpret not having any constraint expressions at all to mean.
	// if !gps.IsAny(pp.Constraint) && !gps.IsNone(pp.Constraint) {
	if !gps.IsAny(project.Constraint) && project.Constraint != nil {
		// Has to be a semver range.
		raw.Version = project.Constraint.String()
	}
	return raw
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
