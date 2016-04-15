package vsolver

import (
	"fmt"

	"github.com/Masterminds/semver"
)

// Version represents one of the different types of versions used by vsolver.
//
// Version is an interface, but it contains private methods, which restricts it
// to vsolver's own internal implementations. We do this for the confluence of
// two reasons:
// - the implementation of Versions is complete (there is no case in which we'd
//   need other types)
// - the implementation relies on type magic under the hood, which would
//   be unsafe to do if other dynamic types could be hiding behind the interface.
type Version interface {
	// Version composes Stringer to ensure that all versions can be serialized
	// to a string
	fmt.Stringer
	_private()
}

// VersionPair represents a normal Version, but paired with its corresponding,
// underlying Revision.
type VersionPair interface {
	Version
	Underlying() Revision
	_pair(int)
}

// UnpairedVersion represents a normal Version, with a method for creating a
// VersionPair by indicating the version's corresponding, underlying Revision.
type UnpairedVersion interface {
	Version
	Is(Revision) VersionPair
	_pair(bool)
}

func (floatingVersion) _private()  {}
func (floatingVersion) _pair(bool) {}
func (plainVersion) _private()     {}
func (plainVersion) _pair(bool)    {}
func (semverVersion) _private()    {}
func (semverVersion) _pair(bool)   {}
func (versionPair) _private()      {}
func (versionPair) _pair(int)      {}
func (Revision) _private()         {}

// NewFloatingVersion creates a new Version to represent a floating version (in
// general, a branch).
func NewFloatingVersion(body string) UnpairedVersion {
	return floatingVersion(body)
}

// NewVersion creates a Semver-typed Version if the provided version string is
// valid semver, and a plain/non-semver version if not.
func NewVersion(body string) UnpairedVersion {
	sv, err := semver.NewVersion(body)

	if err != nil {
		return plainVersion(body)
	}
	return semverVersion{sv: sv}
}

// A Revision represents an immutable versioning identifier.
type Revision string

// String converts the Revision back into a string.
func (r Revision) String() string {
	return string(r)
}

// Admits is the Revision acting as a constraint; it checks to see if the provided
// version is the same Revision as itself.
func (r Revision) Matches(v Version) bool {
	switch tv := v.(type) {
	case Revision:
		return r == tv
	case versionPair:
		return r == tv.r
	}

	return false
}

// AdmitsAny is the Revision acting as a constraint; it checks to see if the provided
// version is the same Revision as itself.
func (r Revision) MatchesAny(c Constraint) bool {
	switch tc := c.(type) {
	case Revision:
		return r == tc
	case versionPair:
		return r == tc.r
	}

	return false
}

func (r Revision) Intersect(c Constraint) Constraint {
	switch tc := c.(type) {
	case Revision:
		if r == tc {
			return r
		}
	case versionPair:
		if r == tc.r {
			return r
		}
	}

	return noneConstraint{}
}

type floatingVersion string

func (v floatingVersion) String() string {
	return string(v)
}

func (v floatingVersion) Matches(v2 Version) bool {
	switch tv := v2.(type) {
	case floatingVersion:
		return v == tv
	case versionPair:
		if tv2, ok := tv.v.(floatingVersion); ok {
			return tv2 == v
		}
	}
	return false
}

func (v floatingVersion) MatchesAny(c Constraint) bool {
	switch tc := c.(type) {
	case floatingVersion:
		return v == tc
	case versionPair:
		if tc2, ok := tc.v.(floatingVersion); ok {
			return tc2 == v
		}
	}

	return false
}

func (v floatingVersion) Intersect(c Constraint) Constraint {
	switch tc := c.(type) {
	case floatingVersion:
		if v == tc {
			return v
		}
	case versionPair:
		if tc2, ok := tc.v.(floatingVersion); ok {
			if v == tc2 {
				return v
			}
		}
	}

	return noneConstraint{}
}

func (v floatingVersion) Is(r Revision) VersionPair {
	return versionPair{
		v: v,
		r: r,
	}
}

type plainVersion string

func (v plainVersion) String() string {
	return string(v)
}

func (v plainVersion) Matches(v2 Version) bool {
	switch tv := v2.(type) {
	case plainVersion:
		return v == tv
	case versionPair:
		if tv2, ok := tv.v.(plainVersion); ok {
			return tv2 == v
		}
	}
	return false
}

func (v plainVersion) MatchesAny(c Constraint) bool {
	switch tc := c.(type) {
	case plainVersion:
		return v == tc
	case versionPair:
		if tc2, ok := tc.v.(plainVersion); ok {
			return tc2 == v
		}
	}

	return false
}

func (v plainVersion) Intersect(c Constraint) Constraint {
	switch tc := c.(type) {
	case plainVersion:
		if v == tc {
			return v
		}
	case versionPair:
		if tc2, ok := tc.v.(plainVersion); ok {
			if v == tc2 {
				return v
			}
		}
	}

	return noneConstraint{}
}

func (v plainVersion) Is(r Revision) VersionPair {
	return versionPair{
		v: v,
		r: r,
	}
}

type semverVersion struct {
	sv *semver.Version
}

func (v semverVersion) String() string {
	return v.sv.String()
}

func (v semverVersion) Matches(v2 Version) bool {
	switch tv := v2.(type) {
	case semverVersion:
		return v.sv.Equal(tv.sv)
	case versionPair:
		if tv2, ok := tv.v.(semverVersion); ok {
			return tv2.sv.Equal(v.sv)
		}
	}
	return false
}

func (v semverVersion) MatchesAny(c Constraint) bool {
	switch tc := c.(type) {
	case semverVersion:
		return v.sv.Equal(tc.sv)
	case versionPair:
		if tc2, ok := tc.v.(semverVersion); ok {
			return tc2.sv.Equal(v.sv)
		}
	}

	return false
}

func (v semverVersion) Intersect(c Constraint) Constraint {
	switch tc := c.(type) {
	case semverVersion:
		if v.sv.Equal(tc.sv) {
			return v
		}
	case versionPair:
		if tc2, ok := tc.v.(semverVersion); ok {
			if v.sv.Equal(tc2.sv) {
				return v
			}
		}
	}

	return noneConstraint{}
}

func (v semverVersion) Is(r Revision) VersionPair {
	return versionPair{
		v: v,
		r: r,
	}
}

type versionPair struct {
	v Version
	r Revision
}

func (v versionPair) String() string {
	return v.v.String()
}

func (v versionPair) Underlying() Revision {
	return v.r
}

func (v versionPair) Matches(v2 Version) bool {
	switch tv2 := v2.(type) {
	case versionPair:
		return v.r == tv2.r
	case Revision:
		return v.r == tv2
	}

	switch tv := v.v.(type) {
	case plainVersion:
		if tv.Matches(v2) {
			return true
		}
	case floatingVersion:
		if tv.Matches(v2) {
			return true
		}
	case semverVersion:
		if tv2, ok := v2.(semverVersion); ok {
			if tv.sv.Equal(tv2.sv) {
				return true
			}
		}
	}

	return false
}

func (v versionPair) MatchesAny(c2 Constraint) bool {
	return c2.Matches(v)
}

func (v versionPair) Intersect(c2 Constraint) Constraint {
	switch tv2 := c2.(type) {
	case versionPair:
		if v.r == tv2.r {
			return v.r
		}
	case Revision:
		if v.r == tv2 {
			return v.r
		}
	}

	switch tv := v.v.(type) {
	case plainVersion, floatingVersion:
		if c2.Matches(v) {
			return v
		}
	case semverVersion:
		if tv2, ok := c2.(semverVersion); ok {
			if tv.sv.Equal(tv2.sv) {
				return v
			}
		}
	}

	return noneConstraint{}
}

// compareVersionType is a sort func helper that makes a coarse-grained sorting
// decision based on version type.
//
// Make sure that l and r have already been converted from versionWithImmut (if
// applicable).
func compareVersionType(l, r Version) int {
	// Big fugly double type switch. No reflect, because this can be smack in a hot loop
	switch l.(type) {
	case Revision:
		switch r.(type) {
		case Revision:
			return 0
		case floatingVersion, plainVersion, semverVersion:
			return 1
		default:
			panic("unknown version type")
		}
	case floatingVersion:
		switch r.(type) {
		case Revision:
			return -1
		case floatingVersion:
			return 0
		case plainVersion, semverVersion:
			return 1
		default:
			panic("unknown version type")
		}

	case plainVersion:
		switch r.(type) {
		case Revision, floatingVersion:
			return -1
		case plainVersion:
			return 0
		case semverVersion:
			return 1
		default:
			panic("unknown version type")
		}

	case semverVersion:
		switch r.(type) {
		case Revision, floatingVersion, plainVersion:
			return -1
		case semverVersion:
			return 0
		default:
			panic("unknown version type")
		}
	default:
		panic("unknown version type")
	}
}
