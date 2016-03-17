package vsolver

type SourceManager interface {
	GetProjectInfo(ProjectID) (ProjectInfo, error)
	ListVersions(ProjectIdentifier) ([]ProjectID, error)
	ProjectExists(ProjectIdentifier) bool
}
