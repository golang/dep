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
type V interface {
	// Version composes Stringer to ensure that all versions can be serialized
	// to a string
	fmt.Stringer
	_private()
}

func (floatingVersion) _private()  {}
func (plainVersion) _private()     {}
func (semverVersion) _private()    {}
func (versionWithImmut) _private() {}
func (Revision) _private()         {}

// VersionPair represents a normal Version, but paired with its corresponding,
// underlying Revision.
type VPair interface {
	V
	Underlying() Revision
}

// NewFloatingVersion creates a new Version to represent a floating version (in
// general, a branch).
func NewFloatingVersion(body string) V {
	return floatingVersion(body)
}

// NewVersion creates a Semver-typed Version if the provided version string is
// valid semver, and a plain/non-semver version if not.
func NewVersion(body string) V {
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
func (r Revision) Admits(v V) bool {
	if r2, ok := v.(Revision); ok {
		return r == r2
	}
	return false
}

// AdmitsAny is the Revision acting as a constraint; it checks to see if the provided
// version is the same Revision as itself.
func (r Revision) AdmitsAny(c Constraint) bool {
	if r2, ok := c.(Revision); ok {
		return r == r2
	}
	return false
}

func (r Revision) Intersect(c Constraint) Constraint {
	if r2, ok := c.(Revision); ok {
		if r == r2 {
			return r
		}
	}
	return noneConstraint{}
}

type floatingVersion string

func (v floatingVersion) String() string {
	return string(v)
}

func (v floatingVersion) Admits(v2 V) bool {
	if fv, ok := v2.(floatingVersion); ok {
		return v == fv
	}
	return false
}

func (v floatingVersion) AdmitsAny(c Constraint) bool {
	if fv, ok := c.(floatingVersion); ok {
		return v == fv
	}
	return false
}

func (v floatingVersion) Intersect(c Constraint) Constraint {
	if fv, ok := c.(floatingVersion); ok {
		if v == fv {
			return v
		}
	}
	return noneConstraint{}
}

type plainVersion string

func (v plainVersion) String() string {
	return string(v)
}

func (v plainVersion) Admits(v2 V) bool {
	if fv, ok := v2.(plainVersion); ok {
		return v == fv
	}
	return false
}

func (v plainVersion) AdmitsAny(c Constraint) bool {
	if fv, ok := c.(plainVersion); ok {
		return v == fv
	}
	return false
}

func (v plainVersion) Intersect(c Constraint) Constraint {
	if fv, ok := c.(plainVersion); ok {
		if v == fv {
			return v
		}
	}
	return noneConstraint{}
}

type semverVersion struct {
	sv *semver.Version
}

func (v semverVersion) String() string {
	return v.sv.String()
}

type versionWithImmut struct {
	main  V
	immut Revision
}

func (v versionWithImmut) String() string {
	return v.main.String()
}

func (v versionWithImmut) Underlying() Revision {
	return v.immut
}

// compareVersionType is a sort func helper that makes a coarse-grained sorting
// decision based on version type.
//
// Make sure that l and r have already been converted from versionWithImmut (if
// applicable).
func compareVersionType(l, r V) int {
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

func WithRevision(v V, r Revision) V {
	if v == nil {
		return r
	}

	switch v.(type) {
	case versionWithImmut, Revision:
		panic("canary - no double dipping")
	}

	return versionWithImmut{
		main:  v,
		immut: r,
	}
}
