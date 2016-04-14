package vsolver

type ProjectName string

type Solver interface {
	Solve(root ProjectInfo, toUpgrade []ProjectName) Result
}

type ProjectAtom struct {
	Name    ProjectName
	Version V
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

type Lock interface {
	// Indicates the version of the solver used to generate this lock file
	SolverVersion() string
	// The hash of inputs to the solver that resulted in this lock file
	InputHash() string
	// Returns the identifier for a project in the lock file, or nil if the
	// named project is not present in the lock file
	GetProjectAtom(ProjectName) *ProjectAtom
}

type lockedProject struct {
	Name, Revision, Version string
}
