package vsolver

import (
	"errors"

	"github.com/Masterminds/semver"
)

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

// ProjectExistence values represent the extent to which a project "exists."
type ProjectExistence uint8

const (
	// DoesNotExist indicates that a particular project URI cannot be located,
	// at any level. It is represented as 1, rather than 0, to differentiate it
	// from the zero-value (which is ExistenceUnknown).
	DoesNotExist ProjectExistence = 1 << iota

	// ExistsInLock indicates that a project exists (i.e., is mentioned in) a
	// lock file.
	// TODO not sure if it makes sense to have this IF it's just the source
	// manager's responsibility for putting this together - the implication is
	// that this is the root lock file, right?
	ExistsInLock

	// ExistsInVendor indicates that a project exists in a vendor directory at
	// the predictable location based on import path. It does NOT imply, much
	// less guarantee, any of the following:
	//   - That the code at the expected location under vendor is at the version
	//   given in a lock file
	//   - That the code at the expected location under vendor is from the
	//   expected upstream project at all
	//   - That, if this flag is not present, the project does not exist at some
	//   unexpected/nested location under vendor
	//   - That the full repository history is available. In fact, the
	//   assumption should be that if only this flag is on, the full repository
	//   history is likely not available locally
	//
	// In short, the information encoded in this flag should in no way be
	// construed as exhaustive.
	ExistsInVendor

	// ExistsInCache indicates that a project exists on-disk in the local cache.
	// It does not guarantee that an upstream exists, thus it cannot imply
	// that the cache is at all correct - up-to-date, or even of the expected
	// upstream project repository.
	//
	// Additionally, this refers only to the existence of the local repository
	// itself; it says nothing about the existence or completeness of the
	// separate metadata cache.
	ExistsInCache

	// ExistsUpstream indicates that a project repository was locatable at the
	// path provided by a project's URI (a base import path).
	ExistsUpstream

	// Indicates that the upstream project, in addition to existing, is also
	// accessible.
	//
	// Different hosting providers treat unauthorized access differently:
	// GitHub, for example, returns 404 (or the equivalent) when attempting unauthorized
	// access, whereas BitBucket returns 403 (or 302 login redirect). Thus,
	// while the ExistsUpstream and UpstreamAccessible bits should always only
	// be on or off together when interacting with Github, it is possible that a
	// BitBucket provider might report ExistsUpstream, but not UpstreamAccessible.
	//
	// For most purposes, non-existence and inaccessibility are treated the
	// same, but clearly delineating the two allows slightly improved UX.
	UpstreamAccessible

	// The zero value; indicates that no work has yet been done to determine the
	// existence level of a project.
	ExistenceUnknown ProjectExistence = 0
)

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
	Version  Version
	Packages []string
}

type Version struct {
	// The type of version identifier
	Type VersionType
	// The version identifier itself
	Info   string
	SemVer *semver.Version
}

type ProjectDep struct {
	ProjectID
	Constraint Constraint
}

type Constraint interface {
	Type() ConstraintType
	Body() string
	Allows(Version) bool
	UnionAllowsAny(Constraint) bool
}

// NewConstraint constructs an appropriate Constraint object from the input
// parameters.
func NewConstraint(t ConstraintType, body string) (Constraint, error) {
	switch t {
	case C_Branch, C_Version, C_Revision:
		return basicConstraint{
			typ:  t,
			body: body,
		}, nil
	case C_Semver, C_SemverRange:
		c, err := semver.NewConstraint(body)
		if err != nil {
			return nil, err
		}

		return semverConstraint{
			typ:  t,
			body: body,
			c:    c,
		}, nil
	default:
		return nil, errors.New("Unknown ConstraintType provided")
	}
}

type basicConstraint struct {
	// The type of constraint - version, branch, or revision
	typ ConstraintType
	// The string text of the constraint
	body string
}

func (c basicConstraint) Type() ConstraintType {
	return c.typ
}

func (c basicConstraint) Body() string {
	return c.body
}

func (c basicConstraint) Allows(v Version) bool {
	if VTCTCompat[v.Type]&c.typ == 0 {
		// version and constraint types are incompatible
		return false
	}

	// Branches, normal versions, and revisions all must be exact string matches
	return c.body == v.Info
}

func (c basicConstraint) UnionAllowsAny(c2 Constraint) bool {
	return c2.Type() == c.typ && c2.Body() == c.body
}

type semverConstraint struct {
	// The type of constraint - single semver, or semver range
	typ ConstraintType
	// The string text of the constraint
	body string
	c    *semver.Constraints
}

func (c semverConstraint) Type() ConstraintType {
	return c.typ
}

func (c semverConstraint) Body() string {
	return c.body
}

func (c semverConstraint) Allows(v Version) bool {
	if VTCTCompat[v.Type]&c.typ == 0 {
		// version and constraint types are incompatible
		return false
	}

	return c.c.Check(v.SemVer)
}

func (c semverConstraint) UnionAllowsAny(c2 Constraint) bool {
	if c2.Type()&(C_Semver|C_SemverRange) == 0 {
		// Union only possible if other constraint is semverish
		return false
	}

	// TODO figure out how we're doing these union checks
	return false // FIXME
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

func (pi ProjectInfo) GetDependencies() []ProjectDep {

}

func (pi ProjectInfo) GetDevDependencies() []ProjectDep {

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
	ref                ProjectIdentifier
	pi                 []*ProjectID
	failed             bool
	hasLock, allLoaded bool
	pf                 PackageFetcher
	//avf                func(ProjectIdentifier) ([]*ProjectID, error)
}

//func NewVersionQueue(ref ProjectIdentifier, lockv *ProjectID, avf func(ProjectIdentifier, *ProjectID) []*ProjectID) (*VersionQueue, error) {
func NewVersionQueue(ref ProjectIdentifier, lockv *ProjectID, pf PackageFetcher) (*VersionQueue, error) {
	vq := &VersionQueue{
		ref: ref,
		//avf: avf,
		pf: pf,
	}

	if lockv != nil {
		vq.hasLock = true
		vq.pi = append(vq.pi, lockv)
	} else {
		var err error
		//vq.pi, err = vq.avf(vq.ref, nil)
		// TODO should probably just make the fetcher return semver already, and
		// update ProjectID to suit
		vq.pi, err = vq.pf.ListVersions(vq.ref)
		if err != nil {
			// TODO pushing this error this early entails that we
			// unconditionally deep scan (e.g. vendor), as well as hitting the
			// network.
			return nil, err
		}
		vq.allLoaded = true
	}

	return vq, nil
}

func (vq *VersionQueue) current() *ProjectID {
	if len(vq.pi > 0) {
		return vq.pi[0]
	}

	return nil
}

func (vq *VersionQueue) advance() (err error) {
	// The current version may have failed, but the next one hasn't
	vq.failed = false

	if !vq.allLoaded {
		// Can only get here if no lock was initially provided, so we know we
		// should have that
		lockv := vq.pi[0]

		//vq.pi, err = vq.avf(vq.ref)
		vq.pi, err = vq.pf.ListVersions(vq.ref)
		if err != nil {
			return
		}

		// search for and remove locked version
		// TODO should be able to avoid O(n) here each time...if it matters
		for k, pi := range vq.pi {
			if pi == lockv {
				// GC-safe deletion for slice w/pointer elements
				vq.pi, vq.pi[len(vq.pi)-1] = append(vq.pi[:k], vq.pi[k+1:]...), nil
			}
		}
	}

	if len(vq.pi > 0) {
		vq.pi = vq.pi[1:]
	}

	// normal end of queue. we don't error; it's left to the caller to infer an
	// empty queue w/a subsequent call to current(), which will return nil.
	// TODO this approach kinda...sucks
	return
}
