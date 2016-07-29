package gps

import (
	"encoding/json"
	"fmt"
	"go/build"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/Masterminds/vcs"
)

// Used to compute a friendly filepath from a URL-shaped input
//
// TODO(sdboyer) this is awful. Right?
var sanitizer = strings.NewReplacer(":", "-", "/", "-", "+", "-")

// A SourceManager is responsible for retrieving, managing, and interrogating
// source repositories. Its primary purpose is to serve the needs of a Solver,
// but it is handy for other purposes, as well.
//
// gps's built-in SourceManager, SourceMgr, is intended to be generic and
// sufficient for any purpose. It provides some additional semantics around the
// methods defined here.
type SourceManager interface {
	// RepoExists checks if a repository exists, either upstream or in the
	// SourceManager's central repository cache.
	// TODO(sdboyer) rename to SourceExists
	RepoExists(ProjectIdentifier) (bool, error)

	// ListVersions retrieves a list of the available versions for a given
	// repository name.
	ListVersions(ProjectIdentifier) ([]Version, error)

	// RevisionPresentIn indicates whether the provided Version is present in
	// the given repository.
	RevisionPresentIn(ProjectIdentifier, Revision) (bool, error)

	// ListPackages parses the tree of the Go packages at or below root of the
	// provided ProjectIdentifier, at the provided version.
	ListPackages(ProjectIdentifier, Version) (PackageTree, error)

	// GetManifestAndLock returns manifest and lock information for the provided
	// root import path.
	//
	// gps currently requires that projects be rooted at their repository root,
	// necessitating that the ProjectIdentifier's ProjectRoot must also be a
	// repository root.
	GetManifestAndLock(ProjectIdentifier, Version) (Manifest, Lock, error)

	// ExportProject writes out the tree of the provided import path, at the
	// provided version, to the provided directory.
	ExportProject(ProjectIdentifier, Version, string) error

	// AnalyzerInfo reports the name and version of the logic used to service
	// GetManifestAndLock().
	AnalyzerInfo() (name string, version *semver.Version)
}

// A ProjectAnalyzer is responsible for analyzing a given path for Manifest and
// Lock information. Tools relying on gps must implement one.
type ProjectAnalyzer interface {
	// Perform analysis of the filesystem tree rooted at path, with the
	// root import path importRoot, to determine the project's constraints, as
	// indicated by a Manifest and Lock.
	DeriveManifestAndLock(path string, importRoot ProjectRoot) (Manifest, Lock, error)
	// Report the name and version of this ProjectAnalyzer.
	Info() (name string, version *semver.Version)
}

// SourceMgr is the default SourceManager for gps.
//
// There's no (planned) reason why it would need to be reimplemented by other
// tools; control via dependency injection is intended to be sufficient.
type SourceMgr struct {
	cachedir string
	pms      map[string]*pmState
	an       ProjectAnalyzer
	ctx      build.Context
}

var _ SourceManager = &SourceMgr{}

// Holds a projectManager, caches of the managed project's data, and information
// about the freshness of those caches
type pmState struct {
	pm   *projectManager
	cf   *os.File // handle for the cache file
	vcur bool     // indicates that we've called ListVersions()
}

// NewSourceManager produces an instance of gps's built-in SourceManager. It
// takes a cache directory (where local instances of upstream repositories are
// stored), a vendor directory for the project currently being worked on, and a
// force flag indicating whether to overwrite the global cache lock file (if
// present).
//
// The returned SourceManager aggressively caches information wherever possible.
// It is recommended that, if tools need to do preliminary, work involving
// upstream repository analysis prior to invoking a solve run, that they create
// this SourceManager as early as possible and use it to their ends. That way,
// the solver can benefit from any caches that may have already been warmed.
//
// gps's SourceManager is intended to be threadsafe (if it's not, please
// file a bug!). It should certainly be safe to reuse from one solving run to
// the next; however, the fact that it takes a basedir as an argument makes it
// much less useful for simultaneous use by separate solvers operating on
// different root projects. This architecture may change in the future.
func NewSourceManager(an ProjectAnalyzer, cachedir string, force bool) (*SourceMgr, error) {
	if an == nil {
		return nil, fmt.Errorf("a ProjectAnalyzer must be provided to the SourceManager")
	}

	err := os.MkdirAll(filepath.Join(cachedir, "sources"), 0777)
	if err != nil {
		return nil, err
	}

	glpath := path.Join(cachedir, "sm.lock")
	_, err = os.Stat(glpath)
	if err == nil && !force {
		return nil, fmt.Errorf("cache lock file %s exists - another process crashed or is still running?", glpath)
	}

	_, err = os.OpenFile(glpath, os.O_CREATE|os.O_RDONLY, 0700) // is 0700 sane for this purpose?
	if err != nil {
		return nil, fmt.Errorf("failed to create global cache lock file at %s with err %s", glpath, err)
	}

	ctx := build.Default
	// Replace GOPATH with our cache dir
	ctx.GOPATH = cachedir

	return &SourceMgr{
		cachedir: cachedir,
		pms:      make(map[string]*pmState),
		ctx:      ctx,
		an:       an,
	}, nil
}

// Release lets go of any locks held by the SourceManager.
func (sm *SourceMgr) Release() {
	os.Remove(path.Join(sm.cachedir, "sm.lock"))
}

// AnalyzerInfo reports the name and version of the injected ProjectAnalyzer.
func (sm *SourceMgr) AnalyzerInfo() (name string, version *semver.Version) {
	return sm.an.Info()
}

// GetManifestAndLock returns manifest and lock information for the provided
// import path. gps currently requires that projects be rooted at their
// repository root, necessitating that the ProjectIdentifier's ProjectRoot must
// also be a repository root.
//
// The work of producing the manifest and lock is delegated to the injected
// ProjectAnalyzer's DeriveManifestAndLock() method.
func (sm *SourceMgr) GetManifestAndLock(id ProjectIdentifier, v Version) (Manifest, Lock, error) {
	pmc, err := sm.getProjectManager(id)
	if err != nil {
		return nil, nil, err
	}

	return pmc.pm.GetInfoAt(v)
}

// ListPackages parses the tree of the Go packages at and below the ProjectRoot
// of the given ProjectIdentifier, at the given version.
func (sm *SourceMgr) ListPackages(id ProjectIdentifier, v Version) (PackageTree, error) {
	pmc, err := sm.getProjectManager(id)
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
// This list is always retrieved from upstream on the first call. Subsequent
// calls will return a cached version of the first call's results. if upstream
// is not accessible (network outage, access issues, or the resource actually
// went away), an error will be returned.
func (sm *SourceMgr) ListVersions(id ProjectIdentifier) ([]Version, error) {
	pmc, err := sm.getProjectManager(id)
	if err != nil {
		// TODO(sdboyer) More-er proper-er errors
		return nil, err
	}

	return pmc.pm.ListVersions()
}

// RevisionPresentIn indicates whether the provided Revision is present in the given
// repository.
func (sm *SourceMgr) RevisionPresentIn(id ProjectIdentifier, r Revision) (bool, error) {
	pmc, err := sm.getProjectManager(id)
	if err != nil {
		// TODO(sdboyer) More-er proper-er errors
		return false, err
	}

	return pmc.pm.RevisionPresentIn(r)
}

// RepoExists checks if a repository exists, either upstream or in the cache,
// for the provided ProjectIdentifier.
func (sm *SourceMgr) RepoExists(id ProjectIdentifier) (bool, error) {
	pms, err := sm.getProjectManager(id)
	if err != nil {
		return false, err
	}

	return pms.pm.CheckExistence(existsInCache) || pms.pm.CheckExistence(existsUpstream), nil
}

// ExportProject writes out the tree of the provided ProjectIdentifier's
// ProjectRoot, at the provided version, to the provided directory.
func (sm *SourceMgr) ExportProject(id ProjectIdentifier, v Version, to string) error {
	pms, err := sm.getProjectManager(id)
	if err != nil {
		return err
	}

	return pms.pm.ExportVersionTo(v, to)
}

// getProjectManager gets the project manager for the given ProjectIdentifier.
//
// If no such manager yet exists, it attempts to create one.
func (sm *SourceMgr) getProjectManager(id ProjectIdentifier) (*pmState, error) {
	// TODO(sdboyer) finish this, it's not sufficient (?)
	n := id.netName()
	var sn string

	// Early check to see if we already have a pm in the cache for this net name
	if pm, exists := sm.pms[n]; exists {
		return pm, nil
	}

	// Figure out the remote repo path
	rr, err := deduceRemoteRepo(n)
	if err != nil {
		// Not a valid import path, must reject
		// TODO(sdboyer) wrap error
		return nil, err
	}

	// Check the cache again, see if exact resulting clone url is in there
	if pm, exists := sm.pms[rr.CloneURL.String()]; exists {
		// Found it - re-register this PM at the original netname so that it
		// doesn't need to deduce next time
		// TODO(sdboyer) is this OK to do? are there consistency side effects?
		sm.pms[n] = pm
		return pm, nil
	}

	// No luck again. Now, walk through the scheme options the deducer returned,
	// checking if each is in the cache
	for _, scheme := range rr.Schemes {
		rr.CloneURL.Scheme = scheme
		// See if THIS scheme has a match, now
		if pm, exists := sm.pms[rr.CloneURL.String()]; exists {
			// Yep - again, re-register this PM at the original netname so that it
			// doesn't need to deduce next time
			// TODO(sdboyer) is this OK to do? are there consistency side effects?
			sm.pms[n] = pm
			return pm, nil
		}
	}

	// Definitively no match for anything in the cache, so we know we have to
	// create the entry. Next question is whether there's already a repo on disk
	// for any of the schemes, or if we need to create that, too.

	// TODO(sdboyer) this strategy kinda locks in the scheme to use over
	// multiple invocations in a way that maybe isn't the best.
	var r vcs.Repo
	for _, scheme := range rr.Schemes {
		rr.CloneURL.Scheme = scheme
		url := rr.CloneURL.String()
		sn := sanitizer.Replace(url)
		path := filepath.Join(sm.cachedir, "sources", sn)

		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			// This one exists, so set up here
			r, err = vcs.NewRepo(url, path)
			if err != nil {
				return nil, err
			}
			goto decided
		}
	}

	// Nothing on disk, either. Iterate through the schemes, trying each and
	// failing out only if none resulted in successfully setting up the local.
	for _, scheme := range rr.Schemes {
		rr.CloneURL.Scheme = scheme
		url := rr.CloneURL.String()
		sn := sanitizer.Replace(url)
		path := filepath.Join(sm.cachedir, "sources", sn)

		r, err := vcs.NewRepo(url, path)
		if err != nil {
			continue
		}

		// FIXME(sdboyer) cloning the repo here puts it on a blocking path. that
		// aspect of state management needs to be deferred into the
		// projectManager
		err = r.Get()
		if err != nil {
			continue
		}
		goto decided
	}

	// If we've gotten this far, we got some brokeass input.
	return nil, fmt.Errorf("Could not reach source repository for %s", n)

decided:
	// Ensure cache dir exists
	metadir := path.Join(sm.cachedir, "metadata", string(n))
	err = os.MkdirAll(metadir, 0777)
	if err != nil {
		// TODO(sdboyer) be better
		return nil, err
	}

	pms := &pmState{}
	cpath := path.Join(metadir, "cache.json")
	fi, err := os.Stat(cpath)
	var dc *projectDataCache
	if fi != nil {
		pms.cf, err = os.OpenFile(cpath, os.O_RDWR, 0777)
		if err != nil {
			// TODO(sdboyer) be better
			return nil, fmt.Errorf("Err on opening metadata cache file: %s", err)
		}

		err = json.NewDecoder(pms.cf).Decode(dc)
		if err != nil {
			// TODO(sdboyer) be better
			return nil, fmt.Errorf("Err on JSON decoding metadata cache file: %s", err)
		}
	} else {
		// TODO(sdboyer) commented this out for now, until we manage it correctly
		//pms.cf, err = os.Create(cpath)
		//if err != nil {
		//// TODO(sdboyer) be better
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
		an: sm.an,
		dc: dc,
		crepo: &repo{
			rpath: sn,
			r:     r,
		},
	}

	pms.pm = pm
	sm.pms[n] = pms
	return pms, nil
}
