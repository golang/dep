// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sort"

	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
)

const ManifestName = "Gopkg.toml"

type Manifest struct {
	Dependencies gps.ProjectConstraints
	Ovr          gps.ProjectConstraints
	Ignored      []string
	Required     []string
}

type rawManifest struct {
	Dependencies []rawProject `toml:"dependencies,omitempty"`
	Overrides    []rawProject `toml:"overrides,omitempty"`
	Ignored      []string     `toml:"ignored,omitempty"`
	Required     []string     `toml:"required,omitempty"`
}

type rawProject struct {
	Name     string `toml:"name"`
	Branch   string `toml:"branch,omitempty"`
	Revision string `toml:"revision,omitempty"`
	Version  string `toml:"version,omitempty"`
	Source   string `toml:"source,omitempty"`
}

func validateManifest(s string) ([]error, error) {
	var errs []error
	// Load the TomlTree from string
	tree, err := toml.Load(s)
	if err != nil {
		return errs, errors.Wrap(err, "Unable to load TomlTree from string")
	}
	// Convert tree to a map
	manifest := tree.ToMap()

	// Look for unknown fields and collect errors
	for prop, val := range manifest {
		switch prop {
		case "metadata":
			// Check if metadata is of Map type
			if reflect.TypeOf(val).Kind() != reflect.Map {
				errs = append(errs, errors.New("metadata should be a TOML table"))
			}
		case "dependencies", "overrides":
			// Invalid if type assertion fails. Not a TOML array of tables.
			if rawProj, ok := val.([]interface{}); ok {
				// Iterate through each array of tables
				for _, v := range rawProj {
					// Check the individual field's key to be valid
					for key, value := range v.(map[string]interface{}) {
						// Check if the key is valid
						switch key {
						case "name", "branch", "revision", "version", "source":
							// valid key
						case "metadata":
							// Check if metadata is of Map type
							if reflect.TypeOf(value).Kind() != reflect.Map {
								errs = append(errs, fmt.Errorf("metadata in %q should be a TOML table", prop))
							}
						default:
							// unknown/invalid key
							errs = append(errs, fmt.Errorf("Invalid key %q in %q", key, prop))
						}
					}
				}
			} else {
				errs = append(errs, fmt.Errorf("%v should be a TOML array of tables", prop))
			}
		case "ignored", "required":
		default:
			errs = append(errs, fmt.Errorf("Unknown field in manifest: %v", prop))
		}
	}

	return errs, nil
}

func readManifest(r io.Reader) (*Manifest, error) {
	buf := &bytes.Buffer{}
	_, err := buf.ReadFrom(r)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read byte stream")
	}

	// Validate manifest and log warnings
	errs, err := validateManifest(buf.String())
	if err != nil {
		return nil, errors.Wrap(err, "Manifest validation failed")
	}
	for _, e := range errs {
		internal.Logf("WARNING: %v", e)
	}

	raw := rawManifest{}
	err = toml.Unmarshal(buf.Bytes(), &raw)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse the manifest as TOML")
	}

	return fromRawManifest(raw)
}

func fromRawManifest(raw rawManifest) (*Manifest, error) {
	m := &Manifest{
		Dependencies: make(gps.ProjectConstraints, len(raw.Dependencies)),
		Ovr:          make(gps.ProjectConstraints, len(raw.Overrides)),
		Ignored:      raw.Ignored,
		Required:     raw.Required,
	}

	for i := 0; i < len(raw.Dependencies); i++ {
		name, prj, err := toProject(raw.Dependencies[i])
		if err != nil {
			return nil, err
		}
		if _, exists := m.Dependencies[name]; exists {
			return nil, errors.Errorf("multiple dependencies specified for %s, can only specify one", name)
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
		Ignored:      m.Ignored,
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

func (m *Manifest) MarshalTOML() ([]byte, error) {
	raw := m.toRaw()
	result, err := toml.Marshal(raw)
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
	if len(m.Ignored) == 0 {
		return nil
	}

	mp := make(map[string]bool, len(m.Ignored))
	for _, i := range m.Ignored {
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
