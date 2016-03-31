package vsolver

import "github.com/Masterminds/semver"

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

type Revision string
