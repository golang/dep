package vsolver

type ProjectIdentifier string

type Solver interface {
	Solve(rootSpec Spec, rootLock Lock, toUpgrade []ProjectIdentifier) Result
}

// TODO naming lolol
type ProjectID struct {
	ID      ProjectIdentifier
	Version Version
}

type ProjectDep struct {
	ID         ProjectIdentifier
	Constraint Constraint
}

type Dependency struct {
	Depender ProjectID
	Dep      ProjectDep
}

// ProjectInfo holds the spec and lock information for a given ProjectID
type ProjectInfo struct {
	ID ProjectID
	Spec
	Lock
}

type Spec interface {
	ID() ProjectIdentifier
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

type Lock interface {
	// Indicates the version of the solver used to generate this lock file
	SolverVersion() string
	// The hash of inputs to the solver that resulted in this lock file
	InputHash() string
	// Returns the identifier for a project in the lock file, or nil if the
	// named project is not present in the lock file
	GetProjectID(ProjectIdentifier) *ProjectID
}

type lockedProject struct {
	Name, Revision, Version string
}
