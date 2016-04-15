package vsolver

type ConstraintType uint8

const (
	RevisionConstraint ConstraintType = iota
	BranchConstraint
	VersionConstraint
	SemverConstraint
)

// ProjectExistence values represent the extent to which a project "exists."
type ProjectExistence uint8

const (
	// ExistsInLock indicates that a project exists (i.e., is mentioned in) a
	// lock file.
	// TODO not sure if it makes sense to have this IF it's just the source
	// manager's responsibility for putting this together - the implication is
	// that this is the root lock file, right?
	ExistsInLock = 1 << iota

	// ExistsInManifest indicates that a project exists (i.e., is mentioned in)
	// a manifest.
	ExistsInManifest

	// ExistsInVendorRoot indicates that a project exists in a vendor directory
	// at the predictable location based on import path. It does NOT imply, much
	// less guarantee, any of the following:
	//   - That the code at the expected location under vendor is at the version
	//   given in a lock file
	//   - That the code at the expected location under vendor is from the
	//   expected upstream project at all
	//   - That, if this flag is not present, the project does not exist at some
	//   unexpected/nested location under vendor
	//   - That the full repository history is available. In fact, the
	//   assumption should be that if only this flag is on, the full repository
	//   history is likely not available (locally)
	//
	// In short, the information encoded in this flag should not be construed as
	// exhaustive.
	ExistsInVendorRoot

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
)

const (
	// Bitmask for existence levels that are managed by the ProjectManager
	pmexLvls ProjectExistence = ExistsInVendorRoot | ExistsInCache | ExistsUpstream
	// Bitmask for existence levels that are managed by the SourceManager
	smexLvls ProjectExistence = ExistsInLock | ExistsInManifest
)
