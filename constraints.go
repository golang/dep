package vsolver

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver"
)

type Constraint interface {
	fmt.Stringer
	Admits(Version) bool
	AdmitsAny(Constraint) bool
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
		return semverC{c: c}, nil
	default:
		return nil, errors.New("Unknown ConstraintType provided")
	}
}

type semverC struct {
	c semver.Constraint
}

func (c semverC) String() string {
	return c.c.String()
}

func (c semverC) Admits(v Version) bool {
	if sv, ok := v.(semverVersion); ok {
		return c.c.Admits(sv.sv) == nil
	}

	return false
}

func (c semverC) AdmitsAny(c2 Constraint) bool {
	if sc, ok := c2.(semverC); ok {
		return c.c.AdmitsAny(sc.c)
	}

	return false
}

func (c semverC) Intersect(c2 Constraint) Constraint {
	if sc, ok := c2.(semverC); ok {
		i := c.c.Intersect(sc.c)
		if !semver.IsNone(i) {
			return semverC{c: i}
		}
	}

	return noneConstraint{}
}

// anyConstraint is an unbounded constraint - it matches all other types of
// constraints. It mirrors the behavior of the semver package's any type.
type anyConstraint struct{}

func (anyConstraint) String() string {
	return "*"
}

func (anyConstraint) Admits(Version) bool {
	return true
}

func (anyConstraint) AdmitsAny(Constraint) bool {
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

func (noneConstraint) Admits(Version) bool {
	return false
}

func (noneConstraint) AdmitsAny(Constraint) bool {
	return false
}

func (noneConstraint) Intersect(Constraint) Constraint {
	return noneConstraint{}
}
