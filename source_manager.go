package vsolver

import (
	"encoding/json"
	"fmt"
	"go/build"
	"os"
	"path"

	"github.com/Masterminds/vcs"
)

// A SourceManager is responsible for retrieving, managing, and interrogating
// source repositories. Its primary purpose is to serve the needs of a Solver,
// but it is handy for other purposes, as well.
//
// vsolver's built-in SourceManager, accessible via NewSourceManager(), is
// intended to be generic and sufficient for any purpose. It provides some
// additional semantics around the methods defined here.
type SourceManager interface {
	// RepoExists checks if a repository exists, either upstream or in the
	// SourceManager's central repository cache.
	RepoExists(ProjectRoot) (bool, error)

	// VendorCodeExists checks if a code tree exists within the stored vendor
	// directory for the the provided import path name.
	VendorCodeExists(ProjectRoot) (bool, error)

	// ListVersions retrieves a list of the available versions for a given
	// repository name.
	ListVersions(ProjectRoot) ([]Version, error)

	// RevisionPresentIn indicates whether the provided Version is present in the given
	// repository. A nil response indicates the version is valid.
	RevisionPresentIn(ProjectRoot, Revision) (bool, error)

	// ListPackages retrieves a tree of the Go packages at or below the provided
	// import path, at the provided version.
	ListPackages(ProjectRoot, Version) (PackageTree, error)

	// GetProjectInfo returns manifest and lock information for the provided
	// import path. vsolver currently requires that projects be rooted at their
	// repository root, which means that this ProjectRoot must also be a
	// repository root.
	GetProjectInfo(ProjectRoot, Version) (Manifest, Lock, error)

	// ExportProject writes out the tree of the provided import path, at the
	// provided version, to the provided directory.
	ExportProject(ProjectRoot, Version, string) error

	// Release lets go of any locks held by the SourceManager.
	Release()
}

// A ProjectAnalyzer is responsible for analyzing a path for Manifest and Lock
// information. Tools relying on vsolver must implement one.
type ProjectAnalyzer interface {
	GetInfo(build.Context, ProjectRoot) (Manifest, Lock, error)
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
	pms               map[ProjectRoot]*pmState
	an                ProjectAnalyzer
	ctx               build.Context
	//pme               map[ProjectRoot]error
}

// Holds a projectManager, caches of the managed project's data, and information
// about the freshness of those caches
type pmState struct {
	pm   *projectManager
	cf   *os.File // handle for the cache file
	vcur bool     // indicates that we've called ListVersions()
}

// NewSourceManager produces an instance of vsolver's built-in SourceManager. It
// takes a cache directory (where local instances of upstream repositories are
// stored), a base directory for the project currently being worked on, and a
// force flag indicating whether to overwrite the global cache lock file (if
// present).
//
// The returned SourceManager aggressively caches
// information wherever possible. It is recommended that, if tools need to do preliminary,
// work involving upstream repository analysis prior to invoking a solve run,
// that they create this SourceManager as early as possible and use it to their
// ends. That way, the solver can benefit from any caches that may have already
// been warmed.
//
// vsolver's SourceManager is intended to be threadsafe (if it's not, please
// file a bug!). It should certainly be safe to reuse from one solving run to
// the next; however, the fact that it takes a basedir as an argument makes it
// much less useful for simultaneous use by separate solvers operating on
// different root projects. This architecture may change in the future.
func NewSourceManager(an ProjectAnalyzer, cachedir, basedir string, force bool) (SourceManager, error) {
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
		pms:      make(map[ProjectRoot]*pmState),
		ctx:      ctx,
		an:       an,
	}, nil
}

// Release lets go of any locks held by the SourceManager.
//
// This will also call Flush(), which will write any relevant caches to disk.
func (sm *sourceManager) Release() {
	os.Remove(path.Join(sm.cachedir, "sm.lock"))
}

// GetProjectInfo returns manifest and lock information for the provided import
// path. vsolver currently requires that projects be rooted at their repository
// root, which means that this ProjectRoot must also be a repository root.
//
// The work of producing the manifest and lock information is delegated to the
// injected ProjectAnalyzer.
func (sm *sourceManager) GetProjectInfo(n ProjectRoot, v Version) (Manifest, Lock, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		return nil, nil, err
	}

	return pmc.pm.GetInfoAt(v)
}

// ListPackages retrieves a tree of the Go packages at or below the provided
// import path, at the provided version.
func (sm *sourceManager) ListPackages(n ProjectRoot, v Version) (PackageTree, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		return PackageTree{}, err
	}

	return pmc.pm.ListPackages(v)
}

// ListVersions retrieves a list of the available versions for a given
// repository name.
//
// The list is not sorted; while it may be returned in the order that the
// underlying VCS reports version information, no guarantee is made. It is
// expected that the caller either not care about order, or sort the result
// themselves.
//
// This list is always retrieved from upstream; if upstream is not accessible
// (network outage, access issues, or the resource actually went away), an error
// will be returned.
func (sm *sourceManager) ListVersions(n ProjectRoot) ([]Version, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		// TODO More-er proper-er errors
		return nil, err
	}

	return pmc.pm.ListVersions()
}

// RevisionPresentIn indicates whether the provided Revision is present in the given
// repository. A nil response indicates the revision is valid.
func (sm *sourceManager) RevisionPresentIn(n ProjectRoot, r Revision) (bool, error) {
	pmc, err := sm.getProjectManager(n)
	if err != nil {
		// TODO More-er proper-er errors
		return false, err
	}

	return pmc.pm.RevisionPresentIn(r)
}

// VendorCodeExists checks if a code tree exists within the stored vendor
// directory for the the provided import path name.
func (sm *sourceManager) VendorCodeExists(n ProjectRoot) (bool, error) {
	pms, err := sm.getProjectManager(n)
	if err != nil {
		return false, err
	}

	return pms.pm.CheckExistence(existsInVendorRoot), nil
}

func (sm *sourceManager) RepoExists(n ProjectRoot) (bool, error) {
	pms, err := sm.getProjectManager(n)
	if err != nil {
		return false, err
	}

	return pms.pm.CheckExistence(existsInCache) || pms.pm.CheckExistence(existsUpstream), nil
}

// ExportProject writes out the tree of the provided import path, at the
// provided version, to the provided directory.
func (sm *sourceManager) ExportProject(n ProjectRoot, v Version, to string) error {
	pms, err := sm.getProjectManager(n)
	if err != nil {
		return err
	}

	return pms.pm.ExportVersionTo(v, to)
}

// getProjectManager gets the project manager for the given ProjectRoot.
//
// If no such manager yet exists, it attempts to create one.
func (sm *sourceManager) getProjectManager(n ProjectRoot) (*pmState, error) {
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
			Infos:    make(map[Revision]projectInfo),
			Packages: make(map[Revision]PackageTree),
			VMap:     make(map[Version]Revision),
			RMap:     make(map[Revision][]Version),
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
