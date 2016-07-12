package gps

// projectExistence values represent the extent to which a project "exists."
type projectExistence uint8

const (
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
	existsInVendorRoot projectExistence = 1 << iota

	// ExistsInCache indicates that a project exists on-disk in the local cache.
	// It does not guarantee that an upstream exists, thus it cannot imply
	// that the cache is at all correct - up-to-date, or even of the expected
	// upstream project repository.
	//
	// Additionally, this refers only to the existence of the local repository
	// itself; it says nothing about the existence or completeness of the
	// separate metadata cache.
	existsInCache

	// ExistsUpstream indicates that a project repository was locatable at the
	// path provided by a project's URI (a base import path).
	existsUpstream
)
