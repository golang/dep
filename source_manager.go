package vsolver

import (
	"encoding/json"
	"fmt"
	"go/build"
	"os"
	"path"

	"github.com/Masterminds/vcs"
)

type SourceManager interface {
	GetProjectInfo(ProjectName, Version) (ProjectInfo, error)
	ListVersions(ProjectName) ([]Version, error)
	RepoExists(ProjectName) (bool, error)
	VendorCodeExists(ProjectName) (bool, error)
	ExternalReach(ProjectName, Version) (map[string][]string, error)
	ListExternal(ProjectName, Version) ([]string, error)
	ListPackages(ProjectName, Version) (map[string]Package, error)
	ExportProject(ProjectName, Version, string) error
	Release()
	// Flush()
}

// ExistenceError is a specialized error type that, in addition to the standard
// error interface, also indicates the amount of searching for a project's
// existence that has been performed, and what level of existence has been
// ascertained.
//
// ExistenceErrors should *only* be returned if the (lack of) existence of a
// project was the underling cause of the error.
//type ExistenceError interface {
//error
//Existence() (search ProjectExistence, found ProjectExistence)
//}

// sourceManager is the default SourceManager for vsolver.
//
// There's no (planned) reason why it would need to be reimplemented by other
// tools; control via dependency injection is intended to be sufficient.
type sourceManager struct {
	cachedir, basedir string
	pms               map[ProjectName]*pmState
	an                ProjectAnalyzer
	ctx               build.Context
	//pme               map[ProjectName]error
}

// Holds a ProjectManager, caches of the managed project's data, and information
// about the freshness of those caches
type pmState struct {
	pm   ProjectManager
	cf   *os.File // handle for the cache file
	vcur bool     // indicates that we've called ListVersions()
}

func NewSourceManager(cachedir, basedir string, force bool, an ProjectAnalyzer) (SourceManager, error) {
	if an == nil {
		return nil, fmt.Errorf("A ProjectAnalyzer must be provided to the SourceManager.")
	}

	err := os.MkdirAll(cachedir, 0777)
	if err != nil {
		return nil, err
	}

	glpath := path.Join(cachedir, "sm.lock")
	_, err = os.Stat(glpath)
	if err == nil && !force {
		return nil, fmt.Errorf("Another process has locked the cachedir, or crashed without cleaning itself properly. Pass force=true to override.")
	}

	_, err = os.OpenFile(glpath, os.O_CREATE|os.O_RDONLY, 0700) // is 0700 sane for this purpose?
	if err != nil {
		return nil, fmt.Errorf("Failed to create global cache lock file at %s with err %s", glpath, err)
	}

	ctx := build.Default
	// Replace GOPATH with our cache dir
	ctx.GOPATH = cachedir

	return &sourceManager{
		cachedir: cachedir,
		pms:      make(map[ProjectName]*pmState),
		ctx:      ctx,
		an:       an,
	}, nil
	// recovery in a defer to be really proper, though
}

func (sm *sourceManager) Release() {
	os.Remove(path.Join(sm.cachedir, "sm.lock"))
}

func (sm *sourceManager) GetProjectInfo(n ProjectName, v Version) (ProjectInfo, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		return ProjectInfo{}, err
	}

	return pmc.pm.GetInfoAt(v)
}

func (sm *sourceManager) ExternalReach(n ProjectName, v Version) (map[string][]string, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		return nil, err
	}

	return pmc.pm.ExternalReach(v)
}

func (sm *sourceManager) ListExternal(n ProjectName, v Version) ([]string, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		return nil, err
	}

	return pmc.pm.ListExternal(v)
}

func (sm *sourceManager) ListPackages(n ProjectName, v Version) (map[string]Package, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		return nil, err
	}

	return pmc.pm.ListPackages(v)
}

func (sm *sourceManager) ListVersions(n ProjectName) ([]Version, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		// TODO More-er proper-er errors
		return nil, err
	}

	return pmc.pm.ListVersions()
}

func (sm *sourceManager) VendorCodeExists(n ProjectName) (bool, error) {
	pms, err := sm.getProjectManager(n)
	if err != nil {
		return false, err
	}

	return pms.pm.CheckExistence(ExistsInVendorRoot), nil
}

func (sm *sourceManager) RepoExists(n ProjectName) (bool, error) {
	pms, err := sm.getProjectManager(n)
	if err != nil {
		return false, err
	}

	return pms.pm.CheckExistence(ExistsInCache) || pms.pm.CheckExistence(ExistsUpstream), nil
}

func (sm *sourceManager) ExportProject(n ProjectName, v Version, to string) error {
	pms, err := sm.getProjectManager(n)
	if err != nil {
		return err
	}

	return pms.pm.ExportVersionTo(v, to)
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

	repodir := path.Join(sm.cachedir, "src", string(n))
	// TODO be more robust about this
	r, err := vcs.NewRepo("https://"+string(n), repodir)
	if err != nil {
		// TODO be better
		return nil, err
	}
	if !r.CheckLocal() {
		// TODO cloning the repo here puts it on a blocking, and possibly
		// unnecessary path. defer it
		err = r.Get()
		if err != nil {
			// TODO be better
			return nil, err
		}
	}

	// Ensure cache dir exists
	metadir := path.Join(sm.cachedir, "metadata", string(n))
	err = os.MkdirAll(metadir, 0777)
	if err != nil {
		// TODO be better
		return nil, err
	}

	pms := &pmState{}
	cpath := path.Join(metadir, "cache.json")
	fi, err := os.Stat(cpath)
	var dc *projectDataCache
	if fi != nil {
		pms.cf, err = os.OpenFile(cpath, os.O_RDWR, 0777)
		if err != nil {
			// TODO be better
			return nil, fmt.Errorf("Err on opening metadata cache file: %s", err)
		}

		err = json.NewDecoder(pms.cf).Decode(dc)
		if err != nil {
			// TODO be better
			return nil, fmt.Errorf("Err on JSON decoding metadata cache file: %s", err)
		}
	} else {
		// TODO commented this out for now, until we manage it correctly
		//pms.cf, err = os.Create(cpath)
		//if err != nil {
		//// TODO be better
		//return nil, fmt.Errorf("Err on creating metadata cache file: %s", err)
		//}

		dc = &projectDataCache{
			Infos: make(map[Revision]ProjectInfo),
			VMap:  make(map[Version]Revision),
			RMap:  make(map[Revision][]Version),
		}
	}

	pm := &projectManager{
		n:         n,
		ctx:       sm.ctx,
		vendordir: sm.basedir + "/vendor",
		an:        sm.an,
		dc:        dc,
		crepo: &repo{
			rpath: repodir,
			r:     r,
		},
	}

	pms.pm = pm
	sm.pms[n] = pms
	return pms, nil
}
