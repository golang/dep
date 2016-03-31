package vsolver

import (
	"fmt"
	"sync"

	"github.com/Masterminds/semver"
	"github.com/Masterminds/vcs"
)

type SourceManager interface {
	GetProjectInfo(ProjectAtom) (ProjectInfo, error)
	ListVersions(ProjectName) ([]Version, error)
	ProjectExists(ProjectName) bool
}

type ProjectManager interface {
	GetInfoAt(Version) (ProjectInfo, error)
	ListVersions() ([]Version, error)
}

type ProjectAnalyzer interface {
	GetInfo() (ProjectInfo, error)
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

type projectInfo struct {
	name     ProjectName
	atominfo map[Version]ProjectInfo // key should be some 'atom' type - a string, i think
	vmap     map[Version]Version     // value is an atom-version, same as above key
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

	// TODO ensure leading dirs exist
	repo, err := vcs.NewRepo(string(n), fmt.Sprintf("%s/src/%s", sm.cachedir, n))
	if err != nil {
		// TODO be better
		return nil, err
	}

	pm := &projectManager{
		name: n,
		an:   sm.anafac(n),
		repo: repo,
	}

	pms := &pmState{
		pm: pm,
	}
	sm.pms[n] = pms
	return pms, nil
}

type projectManager struct {
	name ProjectName
	mut  sync.RWMutex
	repo vcs.Repo
	ex   ProjectExistence
	an   ProjectAnalyzer
}

func (pm *projectManager) GetInfoAt(v Version) (ProjectInfo, error) {
	pm.mut.Lock()

	err := pm.repo.UpdateVersion(v.Info)
	pm.mut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is checkout/whatever failing")
	}

	pm.mut.RLock()
	i, err := pm.an.GetInfo()
	pm.mut.RUnlock()

	return i, err
}

func (pm *projectManager) ListVersions() (vlist []Version, err error) {
	pm.mut.Lock()

	// TODO rigorously figure out what the existence level changes here are
	err = pm.repo.Update()
	// Write segment is done, so release write lock
	pm.mut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is update failing")
	}

	// And grab a read lock
	pm.mut.RLock()
	defer pm.mut.RUnlock()

	// TODO this is WILDLY inefficient. do better
	tags, err := pm.repo.Tags()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is tags failing")
	}

	for _, tag := range tags {
		ci, err := pm.repo.CommitInfo(tag)
		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			panic("canary - why is commit info failing")
		}

		v := Version{
			Type:       V_Version,
			Info:       tag,
			Underlying: ci.Commit,
		}

		sv, err := semver.NewVersion(tag)
		if err != nil {
			v.SemVer = sv
			v.Type = V_Semver
		}

		vlist = append(vlist, v)
	}

	branches, err := pm.repo.Branches()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is branches failing")
	}

	for _, branch := range branches {
		ci, err := pm.repo.CommitInfo(branch)
		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			panic("canary - why is commit info failing")
		}

		vlist = append(vlist, Version{
			Type:       V_Branch,
			Info:       branch,
			Underlying: ci.Commit,
		})
	}

	return vlist, nil
}
