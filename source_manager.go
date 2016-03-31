package vsolver

import (
	"fmt"

	"github.com/Masterminds/vcs"
)

type SourceManager interface {
	GetProjectInfo(ProjectAtom) (ProjectInfo, error)
	ListVersions(ProjectName) ([]Version, error)
	ProjectExists(ProjectName) bool
}

// ExistenceError is a specialized error type that, in addition to the standard
// error interface, also indicates the amount of searching for a project's
// existence that has been performed, and what level of existence has been
// ascertained.
//
// ExistenceErrors should *only* be returned if the (lack of) existence of a
// project was the underling cause of the error.
type ExistenceError interface {
	error
	Existence() (search ProjectExistence, found ProjectExistence)
}

// sourceManager is the default SourceManager for vsolver.
//
// There's no (planned) reason why it would need to be reimplemented by other
// tools; control via dependency injection is intended to be sufficient.
type sourceManager struct {
	cachedir, basedir string
	pms               map[ProjectName]*pmState
	anafac            func(ProjectName) ProjectAnalyzer
	//pme               map[ProjectName]error
}

// Holds a ProjectManager, caches of the managed project's data, and information
// about the freshness of those caches
type pmState struct {
	pm   ProjectManager
	vcur bool // indicates that we've called ListVersions()
	// TODO deal w/ possible local/upstream desync on PAs (e.g., tag moved)
	vlist []Version // TODO temporary until we have a coherent, overall cache structure
}

func NewSourceManager(cachedir, basedir string) (SourceManager, error) {
	// TODO try to create dir if doesn't exist
	return &sourceManager{
		cachedir: cachedir,
		pms:      make(map[ProjectName]*pmState),
	}, nil

	// TODO drop file lock on cachedir somewhere, here. Caller needs a panic
	// recovery in a defer to be really proper, though
}

func (sm *sourceManager) GetProjectInfo(pa ProjectAtom) (ProjectInfo, error) {
	pmc, err := sm.getProjectManager(pa.Name)
	if err != nil {
		return ProjectInfo{}, err
	}

	return pmc.pm.GetInfoAt(pa.Version)
}

func (sm *sourceManager) ListVersions(n ProjectName) ([]Version, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		// TODO More-er proper-er errors
		return nil, err
	}

	if !pmc.vcur {
		pmc.vlist, err = pmc.pm.ListVersions()
		// TODO this perhaps-expensively retries in the failure case
		if err != nil {
			pmc.vcur = true
		}
	}

	return pmc.vlist, err
}

func (sm *sourceManager) ProjectExists(n ProjectName) bool {
	panic("not implemented")
}

// getProjectManager gets the project manager for the given ProjectName.
//
// If no such manager yet exists, it attempts to create one.
func (sm *sourceManager) getProjectManager(n ProjectName) (*pmState, error) {
	// Check pm cache and errcache first
	if pm, exists := sm.pms[n]; exists {
		return pm, nil
		//} else if pme, errexists := sm.pme[name]; errexists {
		//return nil, pme
	}

	path := fmt.Sprintf("%s/src/%s", sm.cachedir, n)
	r, err := vcs.NewRepo(string(n), path)
	if err != nil {
		// TODO be better
		return nil, err
	}

	pm := &projectManager{
		name: n,
		an:   sm.anafac(n),
		crepo: &repo{
			rpath: fmt.Sprintf("%s/src/%s", sm.cachedir, n),
			r:     r,
		},
	}

	pms := &pmState{
		pm: pm,
	}
	sm.pms[n] = pms
	return pms, nil
}
