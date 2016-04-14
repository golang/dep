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
	GetProjectInfo(ProjectAtom) (ProjectInfo, error)
	ListVersions(ProjectName) ([]V, error)
	RepoExists(ProjectName) (bool, error)
	VendorCodeExists(ProjectName) (bool, error)
	ExportAtomTo(ProjectAtom, string) error
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
}

func NewSourceManager(cachedir, basedir string, upgrade, force bool, an ProjectAnalyzer) (SourceManager, error) {
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
		sortup:   upgrade,
		ctx:      ctx,
		an:       an,
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

func (sm *sourceManager) ListVersions(n ProjectName) ([]V, error) {
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

func (sm *sourceManager) ExportAtomTo(pa ProjectAtom, to string) error {
	pms, err := sm.getProjectManager(pa.Name)
	if err != nil {
		return err
	}

	return pms.pm.ExportVersionTo(pa.Version, to)
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
			VMap:  make(map[V]Revision),
			RMap:  make(map[Revision][]V),
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

type upgradeVersionSorter []V
type downgradeVersionSorter []V

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

	switch compareVersionType(l, r) {
	case -1:
		return false
	case 1:
		return true
	case 0:
		break
	default:
		panic("unreachable")
	}

	switch l.(type) {
	// For these, now nothing to do but alpha sort
	case immutableVersion, floatingVersion, plainVersion:
		return l.String() > r.String()
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lsv, rsv := l.(semverVersion).sv, r.(semverVersion).sv
	lpre, rpre := lsv.Prerelease() == "", rsv.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return lsv.GreaterThan(rsv)
}

func (vs downgradeVersionSorter) Less(i, j int) bool {
	l, r := vs[i], vs[j]

	switch compareVersionType(l, r) {
	case -1:
		return false
	case 1:
		return true
	case 0:
		break
	default:
		panic("unreachable")
	}

	switch l.(type) {
	// For these, now nothing to do but alpha
	case immutableVersion, floatingVersion, plainVersion:
		return l.String() < r.String()
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lsv, rsv := l.(semverVersion).sv, r.(semverVersion).sv
	lpre, rpre := lsv.Prerelease() == "", rsv.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return lsv.LessThan(rsv)
}
