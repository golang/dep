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
	// Mutex controlling general access to the cache repo
	crmut sync.RWMutex
	// Object for controlling the cachce repo
	crepo vcs.Repo
	// Whether or not the cache repo has been synced (think dvcs) with upstream
	synced bool
	ex     ProjectExistence
	// Analyzer, created from the injected factory
	an       ProjectAnalyzer
	atominfo map[Version]ProjectInfo // key should be some 'atom' type - a string, i think
	vmap     map[Version]Version     // value is an atom-version, same as above key
}

func (pm *projectManager) GetInfoAt(v Version) (ProjectInfo, error) {
	pm.crmut.Lock()

	err := pm.crepo.UpdateVersion(v.Info)
	pm.crmut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is checkout/whatever failing")
	}

	pm.crmut.RLock()
	i, err := pm.an.GetInfo()
	pm.crmut.RUnlock()

	return i, err
}

func (pm *projectManager) ListVersions() (vlist []Version, err error) {
	pm.crmut.Lock()

	// TODO rigorously figure out what the existence level changes here are
	err = pm.crepo.Update()
	// Write segment is done, so release write lock
	pm.crmut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is update failing")
	}

	// And grab a read lock
	pm.crmut.RLock()
	defer pm.crmut.RUnlock()

	// TODO this is WILDLY inefficient. do better
	tags, err := pm.crepo.Tags()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is tags failing")
	}

	for _, tag := range tags {
		ci, err := pm.crepo.CommitInfo(tag)
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

	branches, err := pm.crepo.Branches()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is branches failing")
	}

	for _, branch := range branches {
		ci, err := pm.crepo.CommitInfo(branch)
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
