package vsolver

import (
	"errors"

	"github.com/Masterminds/semver"
)

type Constraint interface {
	Type() ConstraintType
	Body() string
	Admits(Version) bool
	AdmitsAny(Constraint) bool
	Intersect(Constraint) Constraint
}

// NewConstraint constructs an appropriate Constraint object from the input
// parameters.
func NewConstraint(t ConstraintType, body string) (Constraint, error) {
	switch t {
	case C_Branch, C_Version, C_Revision:
		return basicConstraint{
			typ:  t,
			body: body,
		}, nil
	case C_Semver, C_SemverRange:
		c, err := semver.NewConstraint(body)
		if err != nil {
			return nil, err
		}

		return semverConstraint{
			typ:  t,
			body: body,
			c:    c,
		}, nil
	default:
		return nil, errors.New("Unknown ConstraintType provided")
	}
}

type basicConstraint struct {
	// The type of constraint - version, branch, or revision
	typ ConstraintType
	// The string text of the constraint
	body string
}

func (c basicConstraint) Type() ConstraintType {
	return c.typ
}

func (c basicConstraint) Body() string {
	return c.body
}

func (c basicConstraint) Admits(v Version) bool {
	if VTCTCompat[v.Type]&c.typ == 0 {
		// version and constraint types are incompatible
		return false
	}

	// Branches, normal versions, and revisions all must be exact string matches
	return c.body == v.Info
}

func (c basicConstraint) AdmitsAny(c2 Constraint) bool {
	return (c2.Type() == c.typ && c2.Body() == c.body) || c2.AdmitsAny(c)
}

func (c basicConstraint) Intersect(c2 Constraint) Constraint {
	if c.AdmitsAny(c2) {
		return c
	}

	return noneConstraint{}
}

// anyConstraint is an unbounded constraint - it matches all other types of
// constraints.
type anyConstraint struct{}

func (anyConstraint) Type() ConstraintType {
	return C_ExactMatch | C_FlexMatch
}

func (anyConstraint) Body() string {
	return "*"
}

func (anyConstraint) Admits(v Version) bool {
	return true
}

func (anyConstraint) AdmitsAny(Constraint) bool {
	return true
}

func (anyConstraint) Intersect(c Constraint) Constraint {
	return c
}

type semverConstraint struct {
	// The type of constraint - single semver, or semver range
	typ ConstraintType
	// The string text of the constraint
	body string
	c    semver.Constraint
}

func (c semverConstraint) Type() ConstraintType {
	return c.typ
}

func (c semverConstraint) Body() string {
	return c.body
}

func (c semverConstraint) Admits(v Version) bool {
	if VTCTCompat[v.Type]&c.typ == 0 {
		// version and constraint types are incompatible
		return false
	}

	return c.c.Admits(v.SemVer) == nil
}

func (c semverConstraint) AdmitsAny(c2 Constraint) bool {
	if c2.Type()&(C_Semver|C_SemverRange) == 0 {
		// Union only possible if other constraint is semverish
		return false
	}

	return c.c.AdmitsAny(c2.(semverConstraint).c)
}

func (c semverConstraint) Intersect(c2 Constraint) Constraint {
	// TODO This won't actually be OK, long term
	if sv, ok := c2.(semverConstraint); ok {
		i := c.c.Intersect(sv.c)
		if !semver.IsNone(i) {
			return semverConstraint{
				typ:  C_SemverRange, // TODO get rid of the range/non-range distinction
				c:    i,
				body: i.String(), // TODO this is costly - defer it by making it a method
			}
		}
	}

	return noneConstraint{}
}

type noneConstraint struct{}

func (noneConstraint) Type() ConstraintType {
	return C_FlexMatch | C_ExactMatch
}

func (noneConstraint) Body() string {
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
