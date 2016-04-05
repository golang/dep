package vsolver

import (
	"fmt"
	"os"
	"path"
	"sort"
	"sync"

	"github.com/Masterminds/semver"
	"github.com/Masterminds/vcs"
)

type ProjectManager interface {
	GetInfoAt(Version) (ProjectInfo, error)
	ListVersions() ([]Version, error)
	CheckExistence(ProjectExistence) bool
}

type ProjectAnalyzer interface {
	GetInfo() (ProjectInfo, error)
}

type projectManager struct {
	n ProjectName
	// Cache dir and top-level project vendor dir. Basically duplicated from
	// sourceManager.
	cacheroot, vendordir string
	// Object for the cache repository
	crepo *repo
	// Indicates the extent to which we have searched for, and verified, the
	// existence of the project/repo.
	ex existence
	// Analyzer, created from the injected factory
	an ProjectAnalyzer
	// Whether the cache has the latest info on versions
	cvsync bool
	// The list of versions. Kept separate from the data cache because this is
	// accessed in the hot loop; we don't want to rebuild and realloc for it.
	vlist []Version
	// Direction to sort the version list in (true is for upgrade, false for
	// downgrade)
	sortup bool
	// The project metadata cache. This is persisted to disk, for reuse across
	// solver runs.
	dc *projectDataCache
}

type existence struct {
	// The existence levels for which a search/check has been performed
	s ProjectExistence
	// The existence levels verified to be present through searching
	f ProjectExistence
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
	// Technically, we could attempt to return straight from the metadata cache
	// even if the repo cache doesn't exist on disk. But that would allow weird
	// state inconsistencies (cache exists, but no repo...how does that even
	// happen?) that it'd be better to just not allow so that we don't have to
	// think about it elsewhere
	if !pm.CheckExistence(ExistsInCache) {
		return ProjectInfo{}, fmt.Errorf("Project repository cache for %s does not exist", pm.n)
	}

	if pi, exists := pm.dc.Infos[v.Underlying]; exists {
		return pi, nil
	}

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
		pm.ex.s |= ExistsInCache | ExistsUpstream
		pm.vlist, err = pm.crepo.getCurrentVersionPairs()
		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			return nil, err
		}

		pm.ex.f |= ExistsInCache | ExistsUpstream
		pm.cvsync = true

		// Process the version data into the cache
		// TODO detect out-of-sync data as we do this?
		for _, v := range pm.vlist {
			pm.dc.VMap[v] = v.Underlying
			pm.dc.RMap[v.Underlying] = append(pm.dc.RMap[v.Underlying], v)
		}

		// Sort the versions
		// TODO do this as a heap in the original call
		if pm.sortup {
			sort.Sort(upgradeVersionSorter(pm.vlist))
		} else {
			sort.Sort(downgradeVersionSorter(pm.vlist))
		}
	}

	return pm.vlist, nil
}

// CheckExistence provides a direct method for querying existence levels of the
// project. It will only perform actual searches
func (pm *projectManager) CheckExistence(ex ProjectExistence) bool {
	if pm.ex.s&ex != ex {
		if ex&ExistsInVendorRoot != 0 && pm.ex.s&ExistsInVendorRoot == 0 {
			pm.ex.s |= ExistsInVendorRoot

			fi, err := os.Stat(path.Join(pm.vendordir, string(pm.n)))
			if err != nil && fi.IsDir() {
				pm.ex.f |= ExistsInVendorRoot
			}
		}
		if ex&ExistsInCache != 0 && pm.ex.s&ExistsInCache == 0 {
			pm.ex.s |= ExistsInCache
			if pm.crepo.r.CheckLocal() {
				pm.ex.f |= ExistsInCache
			}
		}
		if ex&ExistsUpstream != 0 && pm.ex.s&ExistsUpstream == 0 {
			//pm.ex.s |= ExistsUpstream
			// TODO maybe need a method to do this as cheaply as possible,
			// per-repo type
		}
	}

	return ex&pm.ex.f == ex
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
