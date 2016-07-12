package gps

import (
	"fmt"

	"github.com/Masterminds/semver"
)

var (
	none = noneConstraint{}
	any  = anyConstraint{}
)

// A Constraint provides structured limitations on the versions that are
// admissible for a given project.
//
// As with Version, it has a private method because the gps's internal
// implementation of the problem is complete, and the system relies on type
// magic to operate.
type Constraint interface {
	fmt.Stringer
	// Matches indicates if the provided Version is allowed by the Constraint.
	Matches(Version) bool
	// MatchesAny indicates if the intersection of the Constraint with the
	// provided Constraint would yield a Constraint that could allow *any*
	// Version.
	MatchesAny(Constraint) bool
	// Intersect computes the intersection of the Constraint with the provided
	// Constraint.
	Intersect(Constraint) Constraint
	_private()
}

func (semverConstraint) _private() {}
func (anyConstraint) _private()    {}
func (noneConstraint) _private()   {}

// NewSemverConstraint attempts to construct a semver Constraint object from the
// input string.
//
// If the input string cannot be made into a valid semver Constraint, an error
// is returned.
func NewSemverConstraint(body string) (Constraint, error) {
	c, err := semver.NewConstraint(body)
	if err != nil {
		return nil, err
	}
	return semverConstraint{c: c}, nil
}

type semverConstraint struct {
	c semver.Constraint
}

func (c semverConstraint) String() string {
	return c.c.String()
}

func (c semverConstraint) Matches(v Version) bool {
	switch tv := v.(type) {
	case versionTypeUnion:
		for _, elem := range tv {
			if c.Matches(elem) {
				return true
			}
		}
	case semVersion:
		return c.c.Matches(tv.sv) == nil
	case versionPair:
		if tv2, ok := tv.v.(semVersion); ok {
			return c.c.Matches(tv2.sv) == nil
		}
	}

	return false
}

func (c semverConstraint) MatchesAny(c2 Constraint) bool {
	return c.Intersect(c2) != none
}

func (c semverConstraint) Intersect(c2 Constraint) Constraint {
	switch tc := c2.(type) {
	case anyConstraint:
		return c
	case versionTypeUnion:
		for _, elem := range tc {
			if rc := c.Intersect(elem); rc != none {
				return rc
			}
		}
	case semverConstraint:
		rc := c.c.Intersect(tc.c)
		if !semver.IsNone(rc) {
			return semverConstraint{c: rc}
		}
	case semVersion:
		rc := c.c.Intersect(tc.sv)
		if !semver.IsNone(rc) {
			// If single version intersected with constraint, we know the result
			// must be the single version, so just return it back out
			return c2
		}
	case versionPair:
		if tc2, ok := tc.v.(semVersion); ok {
			rc := c.c.Intersect(tc2.sv)
			if !semver.IsNone(rc) {
				// same reasoning as previous case
				return c2
			}
		}
	}

	return none
}

// IsAny indicates if the provided constraint is the wildcard "Any" constraint.
func IsAny(c Constraint) bool {
	_, ok := c.(anyConstraint)
	return ok
}

// Any returns a constraint that will match anything.
func Any() Constraint {
	return anyConstraint{}
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
	return none
}
