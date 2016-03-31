package vsolver

import (
	"fmt"
	"sync"

	"github.com/Masterminds/semver"
	"github.com/Masterminds/vcs"
)

type ProjectManager interface {
	GetInfoAt(Version) (ProjectInfo, error)
	ListVersions() ([]Version, error)
}

type ProjectAnalyzer interface {
	GetInfo() (ProjectInfo, error)
}

type projectManager struct {
	name ProjectName
	// Object for the cache repository
	crepo *repo
	ex    ProjectExistence
	// Analyzer, created from the injected factory
	an       ProjectAnalyzer
	atominfo map[Revision]ProjectInfo
	vmap     map[Version]Revision
}

type repo struct {
	// Path to the root of the default working copy (NOT the repo itself)
	rpath string
	// Mutex controlling general access to the repo
	mut sync.RWMutex
	// Object for direct repo interaction
	r vcs.Repo
	// Whether or not the cache repo is in sync (think dvcs) with upstream
	synced bool
}

func (pm *projectManager) GetInfoAt(v Version) (ProjectInfo, error) {
	pm.crepo.mut.Lock()

	err := pm.crepo.r.UpdateVersion(v.Info)
	pm.crepo.mut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is checkout/whatever failing")
	}

	pm.crepo.mut.RLock()
	i, err := pm.an.GetInfo()
	pm.crepo.mut.RUnlock()

	return i, err
}

func (pm *projectManager) ListVersions() (vlist []Version, err error) {
	pm.crepo.mut.Lock()

	// TODO rigorously figure out what the existence level changes here are
	err = pm.crepo.r.Update()
	// Write segment is done, so release write lock
	pm.crepo.mut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is update failing")
	}

	// And grab a read lock
	pm.crepo.mut.RLock()
	defer pm.crepo.mut.RUnlock()

	// TODO this is WILDLY inefficient. do better
	tags, err := pm.crepo.r.Tags()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is tags failing")
	}

	for _, tag := range tags {
		ci, err := pm.crepo.r.CommitInfo(tag)
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

	branches, err := pm.crepo.r.Branches()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is branches failing")
	}

	for _, branch := range branches {
		ci, err := pm.crepo.r.CommitInfo(branch)
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
