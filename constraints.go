package vsolver

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver"
)

// A Constraint provides structured limitations on the versions that are
// admissible for a given project.
type Constraint interface {
	fmt.Stringer
	Matches(Version) bool
	MatchesAny(Constraint) bool
	Intersect(Constraint) Constraint
}

// NewConstraint constructs an appropriate Constraint object from the input
// parameters.
func NewConstraint(t ConstraintType, body string) (Constraint, error) {
	switch t {
	case BranchConstraint:
		return floatingVersion(body), nil
	case RevisionConstraint:
		return Revision(body), nil
	case VersionConstraint:
		c, err := semver.NewConstraint(body)
		if err != nil {
			return plainVersion(body), nil
		}
		return semverConstraint{c: c}, nil
	default:
		return nil, errors.New("Unknown ConstraintType provided")
	}
}

type semverConstraint struct {
	c semver.Constraint
}

func (c semverConstraint) String() string {
	return c.c.String()
}

func (c semverConstraint) Matches(v Version) bool {
	switch tv := v.(type) {
	case semverVersion:
		return c.c.Matches(tv.sv) == nil
	case versionPair:
		if tv2, ok := tv.v.(semverVersion); ok {
			return c.c.Matches(tv2.sv) == nil
		}
	}

	return false
}

func (c semverConstraint) MatchesAny(c2 Constraint) bool {
	switch tc := c2.(type) {
	case semverVersion:
		return c.c.MatchesAny(tc.sv)
	case semverConstraint:
		return c.c.MatchesAny(tc.c)
	case versionPair:
		if tc2, ok := tc.v.(semverVersion); ok {
			return c.c.MatchesAny(tc2.sv)
		}
	}

	return false
}

func (c semverConstraint) Intersect(c2 Constraint) Constraint {
	var rc semver.Constraint
	switch tc := c2.(type) {
	case semverVersion:
		rc = c.c.Intersect(tc.sv)
	case semverConstraint:
		rc = c.c.Intersect(tc.c)
	case versionPair:
		if tc2, ok := tc.v.(semverVersion); ok {
			rc = c.c.Intersect(tc2.sv)
		}
	}

	if semver.IsNone(rc) {
		return noneConstraint{}
	}
	return semverConstraint{c: rc}
}

// anyConstraint is an unbounded constraint - it matches all other types of
// constraints. It mirrors the behavior of the semver package's any type.
type anyConstraint struct{}

func (anyConstraint) String() string {
	return "*"
}

func (anyConstraint) Matches(Version) bool {
	return true
}

func (anyConstraint) MatchesAny(Constraint) bool {
	return true
}

func (anyConstraint) Intersect(c Constraint) Constraint {
	return c
}

// noneConstraint is the empty set - it matches no versions. It mirrors the
// behavior of the semver package's none type.
type noneConstraint struct{}

func (noneConstraint) String() string {
	return ""
}

func (noneConstraint) Matches(Version) bool {
	return false
}

func (noneConstraint) MatchesAny(Constraint) bool {
	return false
}

func (noneConstraint) Intersect(Constraint) Constraint {
	return noneConstraint{}
}
