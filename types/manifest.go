package types

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/sdboyer/gps"
)

type Manifest struct {
	Dependencies gps.ProjectConstraints
	Overrides    gps.ProjectConstraints
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

func ReadManifest(r io.Reader) (*Manifest, error) {
	rm := rawManifest{}
	err := json.NewDecoder(r).Decode(&rm)
	if err != nil {
		return nil, err
	}

	m := &Manifest{
		Dependencies: make(gps.ProjectConstraints, len(rm.Dependencies)),
		Overrides:    make(gps.ProjectConstraints, len(rm.Overrides)),
		Ignores:      rm.Ignores,
	}

	for n, pp := range rm.Dependencies {
		m.Dependencies[gps.ProjectRoot(n)], err = toProps(n, pp)
		if err != nil {
			return nil, err
		}
	}

	for n, pp := range rm.Overrides {
		m.Overrides[gps.ProjectRoot(n)], err = toProps(n, pp)
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
