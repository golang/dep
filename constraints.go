package gps

import (
	"fmt"
	"sort"

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

// A ProjectConstraint combines a ProjectIdentifier with a Constraint. It
// indicates that, if packages contained in the ProjectIdentifier enter the
// depgraph, they must do so at a version that is allowed by the Constraint.
type ProjectConstraint struct {
	Ident      ProjectIdentifier
	Constraint Constraint
}

type workingConstraint struct {
	Ident                     ProjectIdentifier
	Constraint                Constraint
	overrNet, overrConstraint bool
}

type ProjectConstraints map[ProjectRoot]ProjectProperties

func pcSliceToMap(l []ProjectConstraint, r ...[]ProjectConstraint) ProjectConstraints {
	final := make(ProjectConstraints)

	for _, pc := range l {
		final[pc.Ident.ProjectRoot] = ProjectProperties{
			NetworkName: pc.Ident.netName(),
			Constraint:  pc.Constraint,
		}
	}

	for _, pcs := range r {
		for _, pc := range pcs {
			if pp, exists := final[pc.Ident.ProjectRoot]; exists {
				// Technically this should be done through a bridge for
				// cross-version-type matching...but this is a one off for root and
				// that's just ridiculous for this.
				pp.Constraint = pp.Constraint.Intersect(pc.Constraint)
				final[pc.Ident.ProjectRoot] = pp
			} else {
				final[pc.Ident.ProjectRoot] = ProjectProperties{
					NetworkName: pc.Ident.netName(),
					Constraint:  pc.Constraint,
				}
			}
		}
	}

	return final
}

func (m ProjectConstraints) asSortedSlice() []ProjectConstraint {
	pcs := make([]ProjectConstraint, len(m))

	k := 0
	for pr, pp := range m {
		pcs[k] = ProjectConstraint{
			Ident: ProjectIdentifier{
				ProjectRoot: pr,
				NetworkName: pp.NetworkName,
			},
			Constraint: pp.Constraint,
		}
		k++
	}

	sort.Stable(sortedConstraints(pcs))
	return pcs
}

// override treats the ProjectConstraints map as an override map, and applies
// overridden values to the input.
//
// A slice of workingConstraint is returned, allowing differentiation between
// values that were or were not overridden.
func (m ProjectConstraints) override(in []ProjectConstraint) (out []workingConstraint) {
	out = make([]workingConstraint, len(in))
	k := 0
	for _, pc := range in {
		wc := workingConstraint{
			Ident:      pc.Ident.normalize(), // necessary to normalize?
			Constraint: pc.Constraint,
		}

		pr := pc.Ident.ProjectRoot
		if pp, has := m[pr]; has {
			// The rule for overrides is that *any* non-zero value for the prop
			// should be considered an override, even if it's equal to what's
			// already there.
			if pp.Constraint != nil {
				wc.Constraint = pp.Constraint
				wc.overrConstraint = true
			}

			if pp.NetworkName != "" {
				wc.Ident.NetworkName = pp.NetworkName
				wc.overrNet = true
			}

		}
		out[k] = wc
		k++
	}

	return
}

type sortedConstraints []ProjectConstraint

func (s sortedConstraints) Len() int {
	return len(s)
}

func (s sortedConstraints) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortedConstraints) Less(i, j int) bool {
	return s[i].Ident.less(s[j].Ident)
}
