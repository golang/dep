package gps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Masterminds/semver"
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
	// SourceExists checks if a repository exists, either upstream or in the
	// SourceManager's central repository cache.
	SourceExists(ProjectIdentifier) (bool, error)

	// SyncSourceFor will attempt to bring all local information about a source
	// fully up to date.
	SyncSourceFor(ProjectIdentifier) error

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

	// DeduceRootProject takes an import path and deduces the corresponding
	// project/source root.
	DeduceProjectRoot(ip string) (ProjectRoot, error)
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
	lf       *os.File
	srcs     map[string]source
	srcmut   sync.RWMutex
	srcfuts  map[string]*unifiedFuture
	srcfmut  sync.RWMutex
	an       ProjectAnalyzer
	dxt      deducerTrie
	rootxt   prTrie
}

type unifiedFuture struct {
	rc, sc chan struct{}
	rootf  stringFuture
	srcf   sourceFuture
}

var _ SourceManager = &SourceMgr{}

// NewSourceManager produces an instance of gps's built-in SourceManager. It
// takes a cache directory (where local instances of upstream repositories are
// stored), and a ProjectAnalyzer that is used to extract manifest and lock
// information from source trees.
//
// The returned SourceManager aggressively caches information wherever possible.
// If tools need to do preliminary work involving upstream repository analysis
// prior to invoking a solve run, it is recommended that they create this
// SourceManager as early as possible and use it to their ends. That way, the
// solver can benefit from any caches that may have already been warmed.
//
// gps's SourceManager is intended to be threadsafe (if it's not, please file a
// bug!). It should be safe to reuse across concurrent solving runs, even on
// unrelated projects.
func NewSourceManager(an ProjectAnalyzer, cachedir string) (*SourceMgr, error) {
	if an == nil {
		return nil, fmt.Errorf("a ProjectAnalyzer must be provided to the SourceManager")
	}

	err := os.MkdirAll(filepath.Join(cachedir, "sources"), 0777)
	if err != nil {
		return nil, err
	}

	glpath := filepath.Join(cachedir, "sm.lock")
	_, err = os.Stat(glpath)
	if err == nil {
		return nil, CouldNotCreateLockError{
			Path: glpath,
			Err:  fmt.Errorf("cache lock file %s exists - another process crashed or is still running?", glpath),
		}
	}

	fi, err := os.OpenFile(glpath, os.O_CREATE|os.O_EXCL, 0600) // is 0600 sane for this purpose?
	if err != nil {
		return nil, CouldNotCreateLockError{
			Path: glpath,
			Err:  fmt.Errorf("err on attempting to create global cache lock: %s", err),
		}
	}

	return &SourceMgr{
		cachedir: cachedir,
		lf:       fi,
		srcs:     make(map[string]source),
		srcfuts:  make(map[string]*unifiedFuture),
		an:       an,
		dxt:      pathDeducerTrie(),
		rootxt:   newProjectRootTrie(),
	}, nil
}

type CouldNotCreateLockError struct {
	Path string
	Err  error
}

func (e CouldNotCreateLockError) Error() string {
	return e.Err.Error()
}

// Release lets go of any locks held by the SourceManager.
func (sm *SourceMgr) Release() {
	sm.lf.Close()
	os.Remove(filepath.Join(sm.cachedir, "sm.lock"))
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
	src, err := sm.getSourceFor(id)
	if err != nil {
		return nil, nil, err
	}

	return src.getManifestAndLock(id.ProjectRoot, v)
}

// ListPackages parses the tree of the Go packages at and below the ProjectRoot
// of the given ProjectIdentifier, at the given version.
func (sm *SourceMgr) ListPackages(id ProjectIdentifier, v Version) (PackageTree, error) {
	src, err := sm.getSourceFor(id)
	if err != nil {
		return PackageTree{}, err
	}

	return src.listPackages(id.ProjectRoot, v)
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
	src, err := sm.getSourceFor(id)
	if err != nil {
		// TODO(sdboyer) More-er proper-er errors
		return nil, err
	}

	return src.listVersions()
}

// RevisionPresentIn indicates whether the provided Revision is present in the given
// repository.
func (sm *SourceMgr) RevisionPresentIn(id ProjectIdentifier, r Revision) (bool, error) {
	src, err := sm.getSourceFor(id)
	if err != nil {
		// TODO(sdboyer) More-er proper-er errors
		return false, err
	}

	return src.revisionPresentIn(r)
}

// SourceExists checks if a repository exists, either upstream or in the cache,
// for the provided ProjectIdentifier.
func (sm *SourceMgr) SourceExists(id ProjectIdentifier) (bool, error) {
	src, err := sm.getSourceFor(id)
	if err != nil {
		return false, err
	}

	return src.checkExistence(existsInCache) || src.checkExistence(existsUpstream), nil
}

// SyncSourceFor will ensure that all local caches and information about a
// source are up to date with any network-acccesible information.
//
// The primary use case for this is prefetching.
func (sm *SourceMgr) SyncSourceFor(id ProjectIdentifier) error {
	src, err := sm.getSourceFor(id)
	if err != nil {
		return err
	}

	return src.syncLocal()
}

// ExportProject writes out the tree of the provided ProjectIdentifier's
// ProjectRoot, at the provided version, to the provided directory.
func (sm *SourceMgr) ExportProject(id ProjectIdentifier, v Version, to string) error {
	src, err := sm.getSourceFor(id)
	if err != nil {
		return err
	}

	return src.exportVersionTo(v, to)
}

// DeduceRootProject takes an import path and deduces the corresponding
// project/source root.
//
// Note that some import paths may require network activity to correctly
// determine the root of the path, such as, but not limited to, vanity import
// paths. (A special exception is written for gopkg.in to minimize network
// activity, as its behavior is well-structured)
func (sm *SourceMgr) DeduceProjectRoot(ip string) (ProjectRoot, error) {
	if prefix, root, has := sm.rootxt.LongestPrefix(ip); has {
		// The non-matching tail of the import path could still be malformed.
		// Validate just that part, if it exists
		if prefix != ip {
			// TODO(sdboyer) commented until i find a proper description of how
			// to validate an import path
			//if !pathvld.MatchString(strings.TrimPrefix(ip, prefix+"/")) {
			//return "", fmt.Errorf("%q is not a valid import path", ip)
			//}
			// There was one, and it validated fine - add it so we don't have to
			// revalidate it later
			sm.rootxt.Insert(ip, root)
		}
		return root, nil
	}

	ft, err := sm.deducePathAndProcess(ip)
	if err != nil {
		return "", err
	}

	r, err := ft.rootf()
	return ProjectRoot(r), err
}

func (sm *SourceMgr) getSourceFor(id ProjectIdentifier) (source, error) {
	//pretty.Println(id.ProjectRoot)
	nn := id.netName()

	sm.srcmut.RLock()
	src, has := sm.srcs[nn]
	sm.srcmut.RUnlock()
	if has {
		return src, nil
	}

	ft, err := sm.deducePathAndProcess(nn)
	if err != nil {
		return nil, err
	}

	// we don't care about the ident here, and the future produced by
	// deducePathAndProcess will dedupe with what's in the sm.srcs map
	src, _, err = ft.srcf()
	return src, err
}

func (sm *SourceMgr) deducePathAndProcess(path string) (*unifiedFuture, error) {
	// Check for an already-existing future in the map first
	sm.srcfmut.RLock()
	ft, exists := sm.srcfuts[path]
	sm.srcfmut.RUnlock()

	if exists {
		return ft, nil
	}

	// Don't have one - set one up.
	df, err := sm.deduceFromPath(path)
	if err != nil {
		return nil, err
	}

	sm.srcfmut.Lock()
	defer sm.srcfmut.Unlock()
	// A bad interleaving could allow two goroutines to make it here for the
	// same path, so we have to re-check existence.
	if ft, exists = sm.srcfuts[path]; exists {
		return ft, nil
	}

	ft = &unifiedFuture{
		rc: make(chan struct{}, 1),
		sc: make(chan struct{}, 1),
	}

	// Rewrap the rootfinding func in another future
	var pr string
	var rooterr error

	// Kick off the func to get root and register it into the rootxt.
	rootf := func() {
		defer close(ft.rc)
		pr, rooterr = df.root()
		if rooterr != nil {
			// Don't cache errs. This doesn't really hurt the solver, and is
			// beneficial for other use cases because it means we don't have to
			// expose any kind of controls for clearing caches.
			return
		}

		tpr := ProjectRoot(pr)
		sm.rootxt.Insert(pr, tpr)
		// It's not harmful if the netname was a URL rather than an
		// import path
		if pr != path {
			// Insert the result into the rootxt twice - once at the
			// root itself, so as to catch siblings/relatives, and again
			// at the exact provided import path (assuming they were
			// different), so that on subsequent calls, exact matches
			// can skip the regex above.
			sm.rootxt.Insert(path, tpr)
		}
	}

	// If deduction tells us this is slow, do it async in its own goroutine;
	// otherwise, we can do it here and give the scheduler a bit of a break.
	if df.rslow {
		go rootf()
	} else {
		rootf()
	}

	// Store a closure bound to the future result on the futTracker.
	ft.rootf = func() (string, error) {
		<-ft.rc
		return pr, rooterr
	}

	// Root future is handled, now build up the source future.
	//
	// First, complete the partialSourceFuture with information the sm has about
	// our cachedir and analyzer
	fut := df.psf(sm.cachedir, sm.an)

	// The maybeSource-trying process is always slow, so keep it async here.
	var src source
	var ident string
	var srcerr error
	go func() {
		defer close(ft.sc)
		src, ident, srcerr = fut()
		if srcerr != nil {
			// Don't cache errs. This doesn't really hurt the solver, and is
			// beneficial for other use cases because it means we don't have
			// to expose any kind of controls for clearing caches.
			return
		}

		sm.srcmut.Lock()
		defer sm.srcmut.Unlock()

		// Check to make sure a source hasn't shown up in the meantime, or that
		// there wasn't already one at the ident.
		var hasi, hasp bool
		var srci, srcp source
		if ident != "" {
			srci, hasi = sm.srcs[ident]
		}
		srcp, hasp = sm.srcs[path]

		// if neither the ident nor the input path have an entry for this src,
		// we're in the simple case - write them both in and we're done
		if !hasi && !hasp {
			sm.srcs[path] = src
			if ident != path && ident != "" {
				sm.srcs[ident] = src
			}
			return
		}

		// Now, the xors.
		//
		// If already present for ident but not for path, copy ident's src
		// to path. This covers cases like a gopkg.in path referring back
		// onto a github repository, where something else already explicitly
		// looked up that same gh repo.
		if hasi && !hasp {
			sm.srcs[path] = srci
			src = srci
		}
		// If already present for path but not for ident, do NOT copy path's
		// src to ident, but use the returned one instead. Really, this case
		// shouldn't occur at all...? But the crucial thing is that the
		// path-based one has already discovered what actual ident of source
		// they want to use, and changing that arbitrarily would have
		// undefined effects.
		if hasp && !hasi && ident != "" {
			sm.srcs[ident] = src
		}

		// If both are present, then assume we're good, and use the path one
		if hasp && hasi {
			// TODO(sdboyer) compare these (somehow? reflect? pointer?) and if they're not the
			// same object, panic
			src = srcp
		}
	}()

	ft.srcf = func() (source, string, error) {
		<-ft.sc
		return src, ident, srcerr
	}

	sm.srcfuts[path] = ft
	return ft, nil
}
