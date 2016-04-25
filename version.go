package vsolver

import "github.com/Masterminds/semver"

// Version represents one of the different types of versions used by vsolver.
//
// Version composes Constraint, because all versions can be used as a constraint
// (where they allow one, and only one, version - themselves), but constraints
// are not necessarily discrete versions.
//
// Version is an interface, but it contains private methods, which restricts it
// to vsolver's own internal implementations. We do this for the confluence of
// two reasons:
// - the implementation of Versions is complete (there is no case in which we'd
//   need other types)
// - the implementation relies on type magic under the hood, which would
//   be unsafe to do if other dynamic types could be hiding behind the interface.
type Version interface {
	Constraint
	// Indicates the type of version - Revision, Branch, Version, or Semver
	Type() string
}

// PairedVersion represents a normal Version, but paired with its corresponding,
// underlying Revision.
type PairedVersion interface {
	Version
	// Underlying returns the immutable Revision that identifies this Version.
	Underlying() Revision
	// Ensures it is impossible to be both a PairedVersion and an
	// UnpairedVersion
	_pair(int)
}

// UnpairedVersion represents a normal Version, with a method for creating a
// VersionPair by indicating the version's corresponding, underlying Revision.
type UnpairedVersion interface {
	Version
	// Is takes the underlying Revision that this (Unpaired)Version corresponds
	// to and unites them into a PairedVersion.
	Is(Revision) PairedVersion
	// Ensures it is impossible to be both a PairedVersion and an
	// UnpairedVersion
	_pair(bool)
}

// types are weird
func (branchVersion) _private()  {}
func (branchVersion) _pair(bool) {}
func (plainVersion) _private()   {}
func (plainVersion) _pair(bool)  {}
func (semVersion) _private()     {}
func (semVersion) _pair(bool)    {}
func (versionPair) _private()    {}
func (versionPair) _pair(int)    {}
func (Revision) _private()       {}

// NewBranch creates a new Version to represent a floating version (in
// general, a branch).
func NewBranch(body string) UnpairedVersion {
	return branchVersion(body)
}

// NewVersion creates a Semver-typed Version if the provided version string is
// valid semver, and a plain/non-semver version if not.
func NewVersion(body string) UnpairedVersion {
	sv, err := semver.NewVersion(body)

	if err != nil {
		return plainVersion(body)
	}
	return semVersion{sv: sv}
}

// A Revision represents an immutable versioning identifier.
type Revision string

// String converts the Revision back into a string.
func (r Revision) String() string {
	return string(r)
}

func (r Revision) Type() string {
	return "rev"
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
	case anyConstraint:
		return true
	case noneConstraint:
		return false
	case Revision:
		return r == tc
	case versionPair:
		return r == tc.r
	}

	return false
}

func (r Revision) Intersect(c Constraint) Constraint {
	switch tc := c.(type) {
	case anyConstraint:
		return r
	case noneConstraint:
		return none
	case Revision:
		if r == tc {
			return r
		}
	case versionPair:
		if r == tc.r {
			return r
		}
	}

	return none
}

type branchVersion string

func (v branchVersion) String() string {
	return string(v)
}

func (r branchVersion) Type() string {
	return "branch"
}

func (v branchVersion) Matches(v2 Version) bool {
	switch tv := v2.(type) {
	case branchVersion:
		return v == tv
	case versionPair:
		if tv2, ok := tv.v.(branchVersion); ok {
			return tv2 == v
		}
	}
	return false
}

func (v branchVersion) MatchesAny(c Constraint) bool {
	switch tc := c.(type) {
	case anyConstraint:
		return true
	case noneConstraint:
		return false
	case branchVersion:
		return v == tc
	case versionPair:
		if tc2, ok := tc.v.(branchVersion); ok {
			return tc2 == v
		}
	}

	return false
}

func (v branchVersion) Intersect(c Constraint) Constraint {
	switch tc := c.(type) {
	case anyConstraint:
		return v
	case noneConstraint:
		return none
	case branchVersion:
		if v == tc {
			return v
		}
	case versionPair:
		if tc2, ok := tc.v.(branchVersion); ok {
			if v == tc2 {
				return v
			}
		}
	}

	return none
}

func (v branchVersion) Is(r Revision) PairedVersion {
	return versionPair{
		v: v,
		r: r,
	}
}

type plainVersion string

func (v plainVersion) String() string {
	return string(v)
}

func (r plainVersion) Type() string {
	return "version"
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
	case anyConstraint:
		return true
	case noneConstraint:
		return false
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
	case anyConstraint:
		return v
	case noneConstraint:
		return none
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

	return none
}

func (v plainVersion) Is(r Revision) PairedVersion {
	return versionPair{
		v: v,
		r: r,
	}
}

type semVersion struct {
	sv *semver.Version
}

func (v semVersion) String() string {
	return v.sv.String()
}

func (r semVersion) Type() string {
	return "semver"
}

func (v semVersion) Matches(v2 Version) bool {
	switch tv := v2.(type) {
	case semVersion:
		return v.sv.Equal(tv.sv)
	case versionPair:
		if tv2, ok := tv.v.(semVersion); ok {
			return tv2.sv.Equal(v.sv)
		}
	}
	return false
}

func (v semVersion) MatchesAny(c Constraint) bool {
	switch tc := c.(type) {
	case anyConstraint:
		return true
	case noneConstraint:
		return false
	case semVersion:
		return v.sv.Equal(tc.sv)
	case versionPair:
		if tc2, ok := tc.v.(semVersion); ok {
			return tc2.sv.Equal(v.sv)
		}
	}

	return false
}

func (v semVersion) Intersect(c Constraint) Constraint {
	switch tc := c.(type) {
	case anyConstraint:
		return v
	case noneConstraint:
		return none
	case semVersion:
		if v.sv.Equal(tc.sv) {
			return v
		}
	case versionPair:
		if tc2, ok := tc.v.(semVersion); ok {
			if v.sv.Equal(tc2.sv) {
				return v
			}
		}
	}

	return none
}

func (v semVersion) Is(r Revision) PairedVersion {
	return versionPair{
		v: v,
		r: r,
	}
}

type versionPair struct {
	v UnpairedVersion
	r Revision
}

func (v versionPair) String() string {
	return v.v.String()
}

func (v versionPair) Type() string {
	return v.v.Type()
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
	case branchVersion:
		if tv.Matches(v2) {
			return true
		}
	case semVersion:
		if tv2, ok := v2.(semVersion); ok {
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
	case anyConstraint:
		return v
	case noneConstraint:
		return none
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
	case plainVersion, branchVersion:
		if c2.Matches(v) {
			return v
		}
	case semVersion:
		if tv2, ok := c2.(semVersion); ok {
			if tv.sv.Equal(tv2.sv) {
				return v
			}
		}
	}

	return none
}

// compareVersionType is a sort func helper that makes a coarse-grained sorting
// decision based on version type.
//
// Make sure that l and r have already been converted from versionPair (if
// applicable).
func compareVersionType(l, r Version) int {
	// Big fugly double type switch. No reflect, because this can be smack in a hot loop
	switch l.(type) {
	case Revision:
		switch r.(type) {
		case Revision:
			return 0
		case branchVersion, plainVersion, semVersion:
			return 1
		default:
			panic("unknown version type")
		}
	case branchVersion:
		switch r.(type) {
		case Revision:
			return -1
		case branchVersion:
			return 0
		case plainVersion, semVersion:
			return 1
		default:
			panic("unknown version type")
		}

	case plainVersion:
		switch r.(type) {
		case Revision, branchVersion:
			return -1
		case plainVersion:
			return 0
		case semVersion:
			return 1
		default:
			panic("unknown version type")
		}

	case semVersion:
		switch r.(type) {
		case Revision, branchVersion, plainVersion:
			return -1
		case semVersion:
			return 0
		default:
			panic("unknown version type")
		}
	default:
		panic("unknown version type")
	}
}
