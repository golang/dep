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

// anyConstraint is an unbounded constraint - it matches all other types of
// constraints.
type anyConstraint struct{}

func (c anyConstraint) Type() ConstraintType {
	return C_ExactMatch | C_FlexMatch
}

func (c anyConstraint) Body() string {
	return "*"
}

func (c anyConstraint) Admits(v Version) bool {
	return true
}

func (c anyConstraint) AdmitsAny(_ Constraint) bool {
	return true
}

type semverConstraint struct {
	// The type of constraint - single semver, or semver range
	typ ConstraintType
	// The string text of the constraint
	body string
	c    *semver.Constraints
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

	return c.c.Check(v.SemVer)
}

func (c semverConstraint) AdmitsAny(c2 Constraint) bool {
	if c2.Type()&(C_Semver|C_SemverRange) == 0 {
		// Union only possible if other constraint is semverish
		return false
	}

	// TODO figure out how we're doing these union checks
	return false // FIXME
}
