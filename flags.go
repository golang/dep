package vsolver

// The type of the version - branch, revision, or version
type VersionType uint8

const (
	V_Revision VersionType = iota
	V_Branch
	V_Version
	V_Semver
)

type ConstraintType uint8

const (
	C_Revision ConstraintType = 1 << iota
	C_Branch
	C_Version
	C_Semver
	C_SemverRange
	C_ExactMatch = C_Revision | C_Branch | C_Version | C_Semver
	C_FlexMatch  = C_SemverRange
)

var VTCTCompat = [...]ConstraintType{
	C_Revision,
	C_Branch,
	C_Version,
	C_Semver | C_SemverRange,
}

type InfoLevel uint

const (
	FromCache InfoLevel = 1 << iota
)

// ProjectExistence values represent the extent to which a project "exists."
type ProjectExistence uint8

const (
	// DoesNotExist indicates that a particular project URI cannot be located,
	// at any level. It is represented as 1, rather than 0, to differentiate it
	// from the zero-value (which is ExistenceUnknown).
	DoesNotExist ProjectExistence = 1 << iota

	// ExistsInLock indicates that a project exists (i.e., is mentioned in) a
	// lock file.
	// TODO not sure if it makes sense to have this IF it's just the source
	// manager's responsibility for putting this together - the implication is
	// that this is the root lock file, right?
	ExistsInLock

	// ExistsInVendor indicates that a project exists in a vendor directory at
	// the predictable location based on import path. It does NOT imply, much
	// less guarantee, any of the following:
	//   - That the code at the expected location under vendor is at the version
	//   given in a lock file
	//   - That the code at the expected location under vendor is from the
	//   expected upstream project at all
	//   - That, if this flag is not present, the project does not exist at some
	//   unexpected/nested location under vendor
	//   - That the full repository history is available. In fact, the
	//   assumption should be that if only this flag is on, the full repository
	//   history is likely not available locally
	//
	// In short, the information encoded in this flag should in no way be
	// construed as exhaustive.
	ExistsInVendor

	// ExistsInCache indicates that a project exists on-disk in the local cache.
	// It does not guarantee that an upstream exists, thus it cannot imply
	// that the cache is at all correct - up-to-date, or even of the expected
	// upstream project repository.
	//
	// Additionally, this refers only to the existence of the local repository
	// itself; it says nothing about the existence or completeness of the
	// separate metadata cache.
	ExistsInCache

	// ExistsUpstream indicates that a project repository was locatable at the
	// path provided by a project's URI (a base import path).
	ExistsUpstream

	// Indicates that the upstream project, in addition to existing, is also
	// accessible.
	//
	// Different hosting providers treat unauthorized access differently:
	// GitHub, for example, returns 404 (or the equivalent) when attempting unauthorized
	// access, whereas BitBucket returns 403 (or 302 login redirect). Thus,
	// while the ExistsUpstream and UpstreamAccessible bits should always only
	// be on or off together when interacting with Github, it is possible that a
	// BitBucket provider might report ExistsUpstream, but not UpstreamAccessible.
	//
	// For most purposes, non-existence and inaccessibility are treated the
	// same, but clearly delineating the two allows slightly improved UX.
	UpstreamAccessible

	// The zero value; indicates that no work has yet been done to determine the
	// existence level of a project.
	ExistenceUnknown ProjectExistence = 0
)
