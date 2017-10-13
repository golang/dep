// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"sync"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
)

// ManifestName is the manifest file name used by dep.
const ManifestName = "Gopkg.toml"

// Errors
var (
	errInvalidConstraint  = errors.New("\"constraint\" must be a TOML array of tables")
	errInvalidOverride    = errors.New("\"override\" must be a TOML array of tables")
	errInvalidRequired    = errors.New("\"required\" must be a TOML list of strings")
	errInvalidIgnored     = errors.New("\"ignored\" must be a TOML list of strings")
	errInvalidProjectRoot = errors.New("ProjectRoot name validation failed")
)

// Manifest holds manifest file data and implements gps.RootManifest.
type Manifest struct {
	Constraints gps.ProjectConstraints
	Ovr         gps.ProjectConstraints
	Ignored     []string
	Required    []string
}

type rawManifest struct {
	Constraints []rawProject `toml:"constraint,omitempty"`
	Overrides   []rawProject `toml:"override,omitempty"`
	Ignored     []string     `toml:"ignored,omitempty"`
	Required    []string     `toml:"required,omitempty"`
}

type rawProject struct {
	Name     string `toml:"name"`
	Branch   string `toml:"branch,omitempty"`
	Revision string `toml:"revision,omitempty"`
	Version  string `toml:"version,omitempty"`
	Source   string `toml:"source,omitempty"`
}

// NewManifest instantiates a new manifest.
func NewManifest() *Manifest {
	return &Manifest{
		Constraints: make(gps.ProjectConstraints),
		Ovr:         make(gps.ProjectConstraints),
	}
}

func validateManifest(s string) ([]error, error) {
	var warns []error
	// Load the TomlTree from string
	tree, err := toml.Load(s)
	if err != nil {
		return warns, errors.Wrap(err, "Unable to load TomlTree from string")
	}
	// Convert tree to a map
	manifest := tree.ToMap()

	// match abbreviated git hash (7chars) or hg hash (12chars)
	abbrevRevHash := regexp.MustCompile("^[a-f0-9]{7}([a-f0-9]{5})?$")
	// Look for unknown fields and collect errors
	for prop, val := range manifest {
		switch prop {
		case "metadata":
			// Check if metadata is of Map type
			if reflect.TypeOf(val).Kind() != reflect.Map {
				warns = append(warns, errors.New("metadata should be a TOML table"))
			}
		case "constraint", "override":
			valid := true
			// Invalid if type assertion fails. Not a TOML array of tables.
			if rawProj, ok := val.([]interface{}); ok {
				// Check element type. Must be a map. Checking one element would be
				// enough because TOML doesn't allow mixing of types.
				if reflect.TypeOf(rawProj[0]).Kind() != reflect.Map {
					valid = false
				}

				if valid {
					// Iterate through each array of tables
					for _, v := range rawProj {
						// Check the individual field's key to be valid
						for key, value := range v.(map[string]interface{}) {
							// Check if the key is valid
							switch key {
							case "name", "branch", "version", "source":
								// valid key
							case "revision":
								if valueStr, ok := value.(string); ok {
									if abbrevRevHash.MatchString(valueStr) {
										warns = append(warns, fmt.Errorf("revision %q should not be in abbreviated form", valueStr))
									}
								}
							case "metadata":
								// Check if metadata is of Map type
								if reflect.TypeOf(value).Kind() != reflect.Map {
									warns = append(warns, fmt.Errorf("metadata in %q should be a TOML table", prop))
								}
							default:
								// unknown/invalid key
								warns = append(warns, fmt.Errorf("Invalid key %q in %q", key, prop))
							}
						}
					}
				}
			} else {
				valid = false
			}

			if !valid {
				if prop == "constraint" {
					return warns, errInvalidConstraint
				}
				if prop == "override" {
					return warns, errInvalidOverride
				}
			}
		case "ignored", "required":
			valid := true
			if rawList, ok := val.([]interface{}); ok {
				// Check element type of the array. TOML doesn't let mixing of types in
				// array. Checking one element would be enough. Empty array is valid.
				if len(rawList) > 0 && reflect.TypeOf(rawList[0]).Kind() != reflect.String {
					valid = false
				}
			} else {
				valid = false
			}

			if !valid {
				if prop == "ignored" {
					return warns, errInvalidIgnored
				}
				if prop == "required" {
					return warns, errInvalidRequired
				}
			}
		default:
			warns = append(warns, fmt.Errorf("Unknown field in manifest: %v", prop))
		}
	}

	return warns, nil
}

// ValidateProjectRoots validates the project roots present in manifest.
func ValidateProjectRoots(c *Ctx, m *Manifest, sm gps.SourceManager) error {
	// Channel to receive all the errors
	errorCh := make(chan error, len(m.Constraints)+len(m.Ovr))

	var wg sync.WaitGroup

	validate := func(pr gps.ProjectRoot) {
		defer wg.Done()
		origPR, err := sm.DeduceProjectRoot(string(pr))
		if err != nil {
			errorCh <- err
		} else if origPR != pr {
			errorCh <- fmt.Errorf("the name for %q should be changed to %q", pr, origPR)
		}
	}

	for pr := range m.Constraints {
		wg.Add(1)
		go validate(pr)
	}
	for pr := range m.Ovr {
		wg.Add(1)
		go validate(pr)
	}

	wg.Wait()
	close(errorCh)

	var valErr error
	if len(errorCh) > 0 {
		valErr = errInvalidProjectRoot
		c.Err.Printf("The following issues were found in Gopkg.toml:\n\n")
		for err := range errorCh {
			c.Err.Println("  ✗", err.Error())
		}
		c.Err.Println()
	}

	return valErr
}

// readManifest returns a Manifest read from r and a slice of validation warnings.
func readManifest(r io.Reader) (*Manifest, []error, error) {
	buf := &bytes.Buffer{}
	_, err := buf.ReadFrom(r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Unable to read byte stream")
	}

	warns, err := validateManifest(buf.String())
	if err != nil {
		return nil, warns, errors.Wrap(err, "Manifest validation failed")
	}

	raw := rawManifest{}
	err = toml.Unmarshal(buf.Bytes(), &raw)
	if err != nil {
		return nil, warns, errors.Wrap(err, "Unable to parse the manifest as TOML")
	}

	m, err := fromRawManifest(raw)
	return m, warns, err
}

func fromRawManifest(raw rawManifest) (*Manifest, error) {
	m := NewManifest()

	m.Constraints = make(gps.ProjectConstraints, len(raw.Constraints))
	m.Ovr = make(gps.ProjectConstraints, len(raw.Overrides))
	m.Ignored = raw.Ignored
	m.Required = raw.Required

	for i := 0; i < len(raw.Constraints); i++ {
		name, prj, err := toProject(raw.Constraints[i])
		if err != nil {
			return nil, err
		}
		if _, exists := m.Constraints[name]; exists {
			return nil, errors.Errorf("multiple dependencies specified for %s, can only specify one", name)
		}
		m.Constraints[name] = prj
	}

	for i := 0; i < len(raw.Overrides); i++ {
		name, prj, err := toProject(raw.Overrides[i])
		if err != nil {
			return nil, err
		}
		if _, exists := m.Ovr[name]; exists {
			return nil, errors.Errorf("multiple overrides specified for %s, can only specify one", name)
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
		pp.Constraint, err = gps.NewSemverConstraintIC(raw.Version)
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
		Constraints: make([]rawProject, 0, len(m.Constraints)),
		Overrides:   make([]rawProject, 0, len(m.Ovr)),
		Ignored:     m.Ignored,
		Required:    m.Required,
	}
	for n, prj := range m.Constraints {
		raw.Constraints = append(raw.Constraints, toRawProject(n, prj))
	}
	sort.Sort(sortedRawProjects(raw.Constraints))

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

// MarshalTOML serializes this manifest into TOML via an intermediate raw form.
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
			raw.Version = v.ImpliedCaretString()
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
		raw.Version = project.Constraint.ImpliedCaretString()
	}
	return raw
}

// DependencyConstraints returns a list of project-level constraints.
func (m *Manifest) DependencyConstraints() gps.ProjectConstraints {
	return m.Constraints
}

// Overrides returns a list of project-level override constraints.
func (m *Manifest) Overrides() gps.ProjectConstraints {
	return m.Ovr
}

// IgnoredPackages returns a set of import paths to ignore.
func (m *Manifest) IgnoredPackages() *pkgtree.IgnoredRuleset {
	return pkgtree.NewIgnoredRuleset(m.Ignored)
}

// HasConstraintsOn checks if the manifest contains either constraints or
// overrides on the provided ProjectRoot.
func (m *Manifest) HasConstraintsOn(root gps.ProjectRoot) bool {
	if _, has := m.Constraints[root]; has {
		return true
	}
	if _, has := m.Ovr[root]; has {
		return true
	}

	return false
}

// RequiredPackages returns a set of import paths to require.
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
