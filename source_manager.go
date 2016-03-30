package vsolver

type SourceManager interface {
	GetProjectInfo(ProjectAtom) (ProjectInfo, error)
	ListVersions(ProjectName) ([]ProjectAtom, error)
	ProjectExists(ProjectName) bool
}

type ProjectManager interface {
	GetProjectInfo() (ProjectInfo, error)
}
