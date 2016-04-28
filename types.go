package vsolver

type ProjectIdentifier struct {
	LocalName   ProjectName
	NetworkName string
}

type ProjectName string

type ProjectAtom struct {
	Name    ProjectName // TODO to ProjectIdentifier
	Version Version
}

var emptyProjectAtom ProjectAtom

type ProjectDep struct {
	Name       ProjectName // TODO to ProjectIdentifier
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
