package vsolver

import (
	"fmt"

	"github.com/Masterminds/semver"
)

type Revision string

func (r Revision) String() string {
	return string(r)
}

func (r Revision) Admits(v V) bool {
	if r2, ok := v.(Revision); ok {
		return r == r2
	}
	return false
}

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
