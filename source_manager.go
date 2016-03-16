package vsolver

type SourceManager interface {
	GetProjectInfo(ProjectIdentifier) (ProjectInfo, error)
	ListVersions(ProjectIdentifier) ([]*ProjectID, error)
	ProjectExists(ProjectIdentifier) bool
}
