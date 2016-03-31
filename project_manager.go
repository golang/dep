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
	// Cache dir and top-level project vendor dir. Basically duplicated from
	// sourceManager.
	cachedir, vendordir string
	// Object for the cache repository
	crepo *repo
	ex    ProjectExistence
	// Analyzer, created from the injected factory
	an ProjectAnalyzer
	// Whether the cache has the latest info on versions
	cvsync bool
	// The list of versions. Kept separate from the data cache because this is
	// accessed in the hot loop; we don't want to rebuild and realloc for it.
	vlist []Version
	// The project metadata cache. This is persisted to disk, for reuse across
	// solver runs.
	dc *projectDataCache
}

// TODO figure out shape of versions, then implement marshaling/unmarshaling
type projectDataCache struct {
	Version string                   `json:"version"` // TODO use this
	Infos   map[Revision]ProjectInfo `json:"infos"`
	VMap    map[Version]Revision     `json:"vmap"`
	RMap    map[Revision][]Version   `json:"rmap"`
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
	if !pm.cvsync {
		pm.vlist, err = pm.crepo.getCurrentVersionPairs()
		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			return nil, err
		}

		pm.cvsync = true

		// Process the version data into the cache
		// TODO detect out-of-sync data as we do this?
		for _, v := range pm.vlist {
			pm.dc.VMap[v] = v.Underlying
			pm.dc.RMap[v.Underlying] = append(pm.dc.RMap[v.Underlying], v)
		}
	}

	return pm.vlist, nil
}

func (r *repo) getCurrentVersionPairs() (vlist []Version, err error) {
	r.mut.Lock()

	// TODO rigorously figure out what the existence level changes here are
	err = r.r.Update()
	// Write segment is done, so release write lock
	r.mut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is update failing")
	}

	// crepo has been synced, mark it as such
	r.synced = true

	// And grab a read lock
	r.mut.RLock()
	defer r.mut.RUnlock()

	// TODO this is WILDLY inefficient. do better
	tags, err := r.r.Tags()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is tags failing")
	}

	for _, tag := range tags {
		ci, err := r.r.CommitInfo(tag)
		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			panic("canary - why is commit info failing")
		}

		v := Version{
			Type:       V_Version,
			Info:       tag,
			Underlying: Revision(ci.Commit),
		}

		sv, err := semver.NewVersion(tag)
		if err != nil {
			v.SemVer = sv
			v.Type = V_Semver
		}

		vlist = append(vlist, v)
	}

	branches, err := r.r.Branches()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is branches failing")
	}

	for _, branch := range branches {
		ci, err := r.r.CommitInfo(branch)
		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			panic("canary - why is commit info failing")
		}

		vlist = append(vlist, Version{
			Type:       V_Branch,
			Info:       branch,
			Underlying: Revision(ci.Commit),
		})
	}

	return vlist, nil
}
