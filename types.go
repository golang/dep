package vsolver

import "container/list"

// The type of the version - branch, revision, or version
type VersionType uint8

const (
	V_Revision VersionType = iota
	V_Branch
	V_Version
	V_Semver
)

type ConstraintType uint8

const (
	C_Revision ConstraintType = 1 << iota
	C_Branch
	C_Version
	C_Semver
	C_SemverRange
)

var VTCTCompat = [...]ConstraintType{
	C_Revision,
	C_Branch,
	C_Version,
	C_Semver | C_SemverRange,
}

type InfoLevel uint

const (
	FromCache InfoLevel = 1 << iota
)

type DepSpec struct {
	Identifier, VersionSpec string
}

type PackageFetcher interface {
	GetProjectInfo(ProjectIdentifier) (ProjectInfo, error)
	ListVersions(ProjectIdentifier) ([]ProjectID, error)
	ProjectExists(ProjectIdentifier) bool
}

type ProjectIdentifier string

type Solver interface {
	Solve(rootSpec Spec, rootLock Lock, toUpgrade []ProjectIdentifier) Result
}

// TODO naming lolol
type ProjectID struct {
	ID       ProjectIdentifier
	Version  string
	Packages []string
}

type ProjectDep struct {
	ProjectID
	Constraint Constraint
}

type Constraint struct {
	// The type of constraint - version, branch, or revision
	Type ConstraintType
	// The string text of the constraint
	Info string
}

func (c Constraint) Allows(version string) bool {

}

func (c Constraint) Intersects(c2 Constraint) bool {

}

type Dependency struct {
	Depender ProjectID
	Dep      ProjectDep
}

// ProjectInfo holds the spec and lock information for a given ProjectID
type ProjectInfo struct {
	ID   ProjectID
	Spec Spec
	Lock Lock
}

type Spec struct {
	ID ProjectIdentifier
}

// TODO define format for lockfile
type Lock struct {
	// The version of the solver used to generate the lock file
	// TODO impl
	Version  string
	Projects []lockedProject
}

func (l Lock) GetProject(id ProjectIdentifier) *ProjectID {

}

type lockedProject struct {
	Name, Revision, Version string
}

// TODO define result structure - should also be interface?
type Result struct {
}

type VersionQueue struct {
	l      *list.List
	rt     VersionType
	ref    ProjectIdentifier
	pi     []*ProjectID
	failed bool
	vf     func(*ProjectID) error
	//pri    *projectRevisionIterator
	//pf  PackageFetcher // vQ may need to grab specific version info, at times
}

func NewVersionQueue(ref ProjectIdentifier, validator func(*ProjectID) error, pi []*ProjectID) *VersionQueue {
	return &VersionQueue{
		ref: ref,
		//pri: pri,
		vf: validator,
	}
}

func (vq *VersionQueue) current() *ProjectID {
	if len(vq.pi > 0) {
		return vq.pi[0]
	}
}

func (vq *VersionQueue) next() bool {
	// The current version may have failed, but the next one hasn't
	vq.failed = false

	// TODO ordering of queue for highest/lowest version choice logic - do it
	// internally here, or is it better elsewhere?
	for k, pi := range vq.pi {
		err := vq.vf(pi)
		if err == nil {
			vq.pi = vq.pi[k:]
			return true
		}
		// TODO keep this err somewhere?
	}

	return false
}

func (vq *VersionQueue) Back() *ProjectID {
	return vq.l.Back().Value.(*ProjectID)
}

func (vq *VersionQueue) Front() *ProjectID {
	return vq.l.Front().Value.(*ProjectID)
}
func (vq *VersionQueue) Len() int {
	return vq.l.Len()
}
func (vq *VersionQueue) InsertAfter(v, mark *ProjectID) bool {
	return nil != vq.l.InsertAfter(v, *list.Element{Value: mark})
}

func (vq *VersionQueue) InsertBefore(v, mark *ProjectID) bool {
	return nil != vq.l.InsertBefore(v, *list.Element{Value: mark})
}

func (vq *VersionQueue) MoveAfter(v, mark *ProjectID)  {}
func (vq *VersionQueue) MoveBefore(v, mark *ProjectID) {}
func (vq *VersionQueue) MoveToBack(v *ProjectID)       {}
func (vq *VersionQueue) MoveToFront(v *ProjectID)      {}
func (vq *VersionQueue) PushBack(v *ProjectID)         {}
func (vq *VersionQueue) PushFront(v *ProjectID)        {}
func (vq *VersionQueue) Remove(v *ProjectID) bool      {}

//func (vq *VersionQueue) PushBackList(other *VersionQueue) {}
//func (vq *VersionQueue) PushFrontList(other *VersionQueue) {}
