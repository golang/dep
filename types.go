package vsolver

type ProjectName string

type Solver interface {
	Solve(root ProjectInfo, toUpgrade []ProjectName) Result
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

// TODO define format for lockfile
type lock struct {
	// The version of the solver used to generate the lock file
	// TODO impl
	Version  string
	Projects []lockedProject
}

// Lock represents data from a lock file (or however the implementing tool
// chooses to store it) at a particular version that is relevant to the
// satisfiability solving process.
//
// In general, the information produced by vsolver on finding a successful
// solution is all that would be necessary to constitute a lock file, though
// tools can mix other information in their files as they choose.
type Lock interface {
	// Indicates the version of the solver used to generate this lock data
	SolverVersion() string
	// The hash of inputs to vsolver that resulted in this lock data
	InputHash() string
	// Returns the identifier for a project in the lock data, or nil if the
	// named project is not present in the lock file
	GetProjectAtom(ProjectName) *ProjectAtom
}

type lockedProject struct {
	Name, Revision, Version string
}
