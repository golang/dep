package vsolver

import "github.com/Masterminds/semver"

type Version struct {
	// The type of version identifier
	Type VersionType
	// The version identifier itself
	Info   string
	SemVer *semver.Version
}
