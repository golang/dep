package vsolver

import (
	"fmt"

	"github.com/Masterminds/semver"
)

var emptyVersion = Version{}

type Version struct {
	// The type of version identifier
	Type VersionType
	// The version identifier itself
	Info string
	// The underlying revision
	Underlying Revision
	SemVer     *semver.Version
}

func (v Version) String() string {
	return v.Info
}

type Revision string

type V interface {
	// Version composes Stringer to ensure that all versions can be serialized
	// to a string
	fmt.Stringer
}

type VPair interface {
	V
	Underlying() Revision
}

type floatingVersion struct {
	body string
}

func (v floatingVersion) String() string {
	return v.body
}

func (v floatingVersion) Admits(v2 V) bool {
	if fv, ok := v2.(floatingVersion); ok {
		return v.body == fv.body
	}
	return false
}

func (v floatingVersion) AdmitsAny(c Constraint) bool {
	if fv, ok := c.(floatingVersion); ok {
		return v.body == fv.body
	}
	return false
}

func (v floatingVersion) Intersect(c Constraint) Constraint {
	if fv, ok := c.(floatingVersion); ok {
		if v.body == fv.body {
			return v
		}
	}
	return noneConstraint{}
}

type plainVersion struct {
	body string
}

func (v plainVersion) String() string {
	return v.body
}

func (v plainVersion) Admits(v2 V) bool {
	if fv, ok := v2.(plainVersion); ok {
		return v.body == fv.body
	}
	return false
}

func (v plainVersion) AdmitsAny(c Constraint) bool {
	if fv, ok := c.(plainVersion); ok {
		return v.body == fv.body
	}
	return false
}

func (v plainVersion) Intersect(c Constraint) Constraint {
	if fv, ok := c.(plainVersion); ok {
		if v.body == fv.body {
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

type immutableVersion struct {
	body string
}

func (v immutableVersion) String() string {
	return v.body
}

func (v immutableVersion) Admits(v2 V) bool {
	if fv, ok := v2.(immutableVersion); ok {
		return v.body == fv.body
	}
	return false
}

func (v immutableVersion) AdmitsAny(c Constraint) bool {
	if fv, ok := c.(immutableVersion); ok {
		return v.body == fv.body
	}
	return false
}

func (v immutableVersion) Intersect(c Constraint) Constraint {
	if fv, ok := c.(immutableVersion); ok {
		if v.body == fv.body {
			return v
		}
	}
	return noneConstraint{}
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

func NewFloatingVersion(body string) V {
	return floatingVersion{body: body}
}

func NewVersion(body string) V {
	sv, err := semver.NewVersion(body)

	if err != nil {
		return plainVersion{body: body}
	}
	return semverVersion{sv: sv}
}

func compareVersionType(l, r V) int {
	// Big fugly double type switch. No reflect, because this can be smack in a hot loop
	switch l.(type) {
	case immutableVersion:
		switch r.(type) {
		case immutableVersion:
			return 0
		case floatingVersion, plainVersion, semverVersion:
			return -1
		default:
			panic("unknown version type")
		}
	case floatingVersion:
		switch r.(type) {
		case immutableVersion:
			return 1
		case floatingVersion:
			return 0
		case plainVersion, semverVersion:
			return -1
		default:
			panic("unknown version type")
		}

	case plainVersion:
		switch r.(type) {
		case immutableVersion, floatingVersion:
			return 1
		case plainVersion:
			return 0
		case semverVersion:
			return -1
		default:
			panic("unknown version type")
		}

	case semverVersion:
		switch r.(type) {
		case immutableVersion, floatingVersion, plainVersion:
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
	switch v.(type) {
	case versionWithImmut, immutableVersion:
		return v
	}

	return versionWithImmut{
		main:  v,
		immut: r,
	}
}
