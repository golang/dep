package vsolver

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/Masterminds/vcs"
)

type SourceManager interface {
	GetProjectInfo(ProjectAtom) (ProjectInfo, error)
	ListVersions(ProjectName) ([]Version, error)
	RepoExists(ProjectName) (bool, error)
	VendorCodeExists(ProjectName) (bool, error)
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
	// Whether to sort versions for upgrade or downgrade
	sortup bool
	//pme               map[ProjectName]error
}

// Holds a ProjectManager, caches of the managed project's data, and information
// about the freshness of those caches
type pmState struct {
	pm   ProjectManager
	cf   *os.File // handle for the cache file
	vcur bool     // indicates that we've called ListVersions()
	// TODO deal w/ possible local/upstream desync on PAs (e.g., tag moved)
	vlist []Version // TODO temporary until we have a coherent, overall cache structure
}

func NewSourceManager(cachedir, basedir string, upgrade, force bool) (SourceManager, error) {
	err := os.MkdirAll(cachedir, 0777)
	if err != nil {
		return nil, err
	}

	glpath := path.Join(cachedir, "sm.lock")
	_, err = os.Stat(glpath)
	if err == nil && !force {
		return nil, fmt.Errorf("Another process has locked the cachedir, or crashed without cleaning itself properly. Pass force=true to override.", err)
	}

	_, err = os.OpenFile(glpath, os.O_CREATE|os.O_RDONLY, 0700) // is 0700 sane for this purpose?
	if err != nil {
		return nil, fmt.Errorf("Failed to create global cache lock file at %s with err %s", glpath, err)
	}

	return &sourceManager{
		cachedir: cachedir,
		pms:      make(map[ProjectName]*pmState),
		sortup:   upgrade,
	}, nil
	// recovery in a defer to be really proper, though
}

func (sm *sourceManager) Release() {
	os.Remove(path.Join(sm.cachedir, "sm.lock"))
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
			return nil, err
		}

		err = json.NewDecoder(pms.cf).Decode(dc)
		if err != nil {
			// TODO be better
			return nil, err
		}
	} else {
		pms.cf, err = os.Create(cpath)
		if err != nil {
			// TODO be better
			return nil, err
		}

		dc = &projectDataCache{
			Infos: make(map[Revision]ProjectInfo),
			VMap:  make(map[Version]Revision),
			RMap:  make(map[Revision][]Version),
		}
	}

	pm := &projectManager{
		n:         n,
		cacheroot: sm.cachedir,
		vendordir: sm.basedir + "/vendor",
		//an:        sm.anafac(n), // TODO
		dc: dc,
		crepo: &repo{
			rpath: repodir,
			r:     r,
		},
	}

	pms.pm = pm
	sm.pms[n] = pms
	return pms, nil
}

type upgradeVersionSorter []Version
type downgradeVersionSorter []Version

func (vs upgradeVersionSorter) Len() int {
	return len(vs)
}

func (vs upgradeVersionSorter) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}

func (vs downgradeVersionSorter) Len() int {
	return len(vs)
}

func (vs downgradeVersionSorter) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}

func (vs upgradeVersionSorter) Less(i, j int) bool {
	l, r := vs[i], vs[j]

	// Start by always sorting higher vtypes earlier
	// TODO need a new means when we get rid of those types
	if l.Type != r.Type {
		return l.Type > r.Type
	}

	switch l.Type {
	case V_Branch, V_Version, V_Revision:
		return l.Info < r.Info
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lpre, rpre := l.SemVer.Prerelease() == "", r.SemVer.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return l.SemVer.GreaterThan(r.SemVer)
}

func (vs downgradeVersionSorter) Less(i, j int) bool {
	l, r := vs[i], vs[j]

	// Start by always sorting higher vtypes earlier
	// TODO need a new means when we get rid of those types
	if l.Type != r.Type {
		return l.Type > r.Type
	}

	switch l.Type {
	case V_Branch, V_Version, V_Revision:
		return l.Info < r.Info
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lpre, rpre := l.SemVer.Prerelease() == "", r.SemVer.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return l.SemVer.LessThan(r.SemVer)
}
