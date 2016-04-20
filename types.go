package vsolver

type ProjectName string

type Solver interface {
	Solve(root ProjectInfo, changeAll bool, toUpgrade []ProjectName) Result
	HashInputs(path string, m Manifest) []byte
}

type ProjectAtom struct {
	Name    ProjectName
	Version Version
}

var emptyProjectAtom ProjectAtom

type ProjectDep struct {
	Name       ProjectName
	Constraint Constraint
}

type Dependency struct {
	Depender ProjectAtom
	Dep      ProjectDep
}

// ProjectInfo holds the spec and lock information for a given ProjectAtom
type ProjectInfo struct {
	pa ProjectAtom
	Manifest
	Lock
}

// LockedProject is a single project entry from a lock file. It expresses the
// project's name, one or both of version and underlying revision, the URI for
// accessing it, and the path at which it should be placed within a vendor
// directory.
//
// TODO note that sometime soon, we also plan to allow pkgs. this'll change
type LockedProject struct {
	n         ProjectName
	v         UnpairedVersion
	r         Revision
	path, uri string
}

// NewLockedProject creates a new LockedProject struct with a given name,
// version, upstream repository URI, and on-disk path at which the project is to
// be checked out under a vendor directory.
//
// Note that passing a nil version will cause a panic. This is a correctness
// measure to ensure that the solver is never exposed to a version-less lock
// entry. Such a case would be meaningless - the solver would have no choice but
// to simply dismiss that project. By creating a hard failure case via panic
// instead, we are trying to avoid inflicting the resulting pain on the user by
// instead forcing a decision on the Analyzer implementation.
func NewLockedProject(n ProjectName, v Version, uri, path string) LockedProject {
	if v == nil {
		panic("must provide a non-nil version to create a LockedProject")
	}

	lp := LockedProject{
		n:    n,
		uri:  uri,
		path: path,
	}

	switch tv := v.(type) {
	case Revision:
		lp.r = tv
	case branchVersion:
		lp.v = tv
	case semVersion:
		lp.v = tv
	case plainVersion:
		lp.v = tv
	case versionPair:
		lp.r = tv.r
		lp.v = tv.v
	}

	return lp
}

// Name returns the name of the locked project.
func (lp LockedProject) Name() ProjectName {
	return lp.n
}

// Version assembles together whatever version and/or revision data is
// available into a single Version.
func (lp LockedProject) Version() Version {
	if lp.r == "" {
		return lp.v
	}

	if lp.v == nil {
		return lp.r
	}

	return lp.v.Is(lp.r)
}

// URI returns the upstream URI of the locked project.
func (lp LockedProject) URI() string {
	return lp.uri
}

// Path returns the path relative to the vendor directory to which the locked
// project should be checked out.
func (lp LockedProject) Path() string {
	return lp.path
}

// TODO undecided on whether having a struct lke this is good/helpful
// PI (Project Info) holds the two key pieces of information that an analyzer
// can produce about a project: a Manifest, describing its intended dependencies
// and certain governing configuration
//type PI struct {
//Manifest
//Lock
////Extra interface{} // TODO allow analyzers to tuck data away if they want
//}

// Manifest represents the data from a manifest file (or however the
// implementing tool chooses to store it) at a particular version that is
// relevant to the satisfiability solving process:
//
// - A list of dependencies: project name, and a constraint
// - A list of development-time dependencies (e.g. for testing - only
// the root project's are incorporated)
//
// Finding a solution that satisfies the constraints expressed by all of these
// dependencies (and those from all other projects, transitively), is what the
// solver does.
//
// Note that vsolver does perform static analysis on all projects' codebases;
// if dependencies it finds through that analysis are missing from what the
// Manifest lists, it is considered an error that will eliminate that version
// from consideration in the solving algorithm.
type Manifest interface {
	Name() ProjectName
	GetDependencies() []ProjectDep
	GetDevDependencies() []ProjectDep
}

// Lock represents data from a lock file (or however the implementing tool
// chooses to store it) at a particular version that is relevant to the
// satisfiability solving process.
//
// In general, the information produced by vsolver on finding a successful
// solution is all that would be necessary to constitute a lock file, though
// tools can include whatever other information they want in their storage.
type Lock interface {
	// Indicates the version of the solver used to generate this lock data
	//SolverVersion() string

	// The hash of inputs to vsolver that resulted in this lock data
	InputHash() string

	// Projects returns the list of LockedProjects contained in the lock data.
	Projects() []LockedProject
}

// SimpleLock is a helper for tools to simply enumerate lock data when they know
// that no hash, or other complex information, is available.
type SimpleLock []LockedProject

// InputHash always returns an empty string for SimpleLock. This makes it useless
// as a stable lock to be written to disk, but still useful for some ephemeral
// purposes.
func (SimpleLock) InputHash() string {
	return ""
}

// Projects returns the entire contents of the SimpleLock.
func (l SimpleLock) Projects() []LockedProject {
	return l
}

// SimpleManifest is a helper for tools to enumerate manifest data. It's
// intended for ephemeral manifests, such as those created by Analyzers on the
// fly.
type SimpleManifest struct {
	N  ProjectName
	P  []ProjectDep
	DP []ProjectDep
}

// Name returns the name of the project described by the manifest.
func (m SimpleManifest) Name() ProjectName {
	return m.N
}

// GetDependencies returns the project's dependencies.
func (m SimpleManifest) GetDependencies() []ProjectDep {
	return m.P
}

// GetDependencies returns the project's test dependencies.
func (m SimpleManifest) GetDevDependencies() []ProjectDep {
	return m.DP
}
