package gps

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// sourceExistence values represent the extent to which a project "exists."
type sourceExistence uint8

const (
	// ExistsInVendorRoot indicates that a project exists in a vendor directory
	// at the predictable location based on import path. It does NOT imply, much
	// less guarantee, any of the following:
	//   - That the code at the expected location under vendor is at the version
	//   given in a lock file
	//   - That the code at the expected location under vendor is from the
	//   expected upstream project at all
	//   - That, if this flag is not present, the project does not exist at some
	//   unexpected/nested location under vendor
	//   - That the full repository history is available. In fact, the
	//   assumption should be that if only this flag is on, the full repository
	//   history is likely not available (locally)
	//
	// In short, the information encoded in this flag should not be construed as
	// exhaustive.
	existsInVendorRoot sourceExistence = 1 << iota

	// ExistsInCache indicates that a project exists on-disk in the local cache.
	// It does not guarantee that an upstream exists, thus it cannot imply
	// that the cache is at all correct - up-to-date, or even of the expected
	// upstream project repository.
	//
	// Additionally, this refers only to the existence of the local repository
	// itself; it says nothing about the existence or completeness of the
	// separate metadata cache.
	existsInCache

	// ExistsUpstream indicates that a project repository was locatable at the
	// path provided by a project's URI (a base import path).
	existsUpstream
)

type source interface {
	syncLocal() error
	checkExistence(sourceExistence) bool
	exportVersionTo(Version, string) error
	getManifestAndLock(ProjectRoot, Version) (Manifest, Lock, error)
	listPackages(ProjectRoot, Version) (PackageTree, error)
	listVersions() ([]Version, error)
	revisionPresentIn(Revision) (bool, error)
}

type sourceMetaCache struct {
	//Version  string                   // TODO(sdboyer) use this
	infos  map[Revision]projectInfo
	ptrees map[Revision]PackageTree
	vMap   map[UnpairedVersion]Revision
	rMap   map[Revision][]UnpairedVersion
	// TODO(sdboyer) mutexes. actually probably just one, b/c complexity
}

// projectInfo holds manifest and lock
type projectInfo struct {
	Manifest
	Lock
}

type existence struct {
	// The existence levels for which a search/check has been performed
	s sourceExistence

	// The existence levels verified to be present through searching
	f sourceExistence
}

func newMetaCache() *sourceMetaCache {
	return &sourceMetaCache{
		infos:  make(map[Revision]projectInfo),
		ptrees: make(map[Revision]PackageTree),
		vMap:   make(map[UnpairedVersion]Revision),
		rMap:   make(map[Revision][]UnpairedVersion),
	}
}

type baseVCSSource struct {
	// Object for the cache repository
	crepo *repo

	// Indicates the extent to which we have searched for, and verified, the
	// existence of the project/repo.
	ex existence

	// ProjectAnalyzer used to fulfill getManifestAndLock
	an ProjectAnalyzer

	// The project metadata cache. This is (or is intended to be) persisted to
	// disk, for reuse across solver runs.
	dc *sourceMetaCache

	// lvfunc allows the other vcs source types that embed this type to inject
	// their listVersions func into the baseSource, for use as needed.
	lvfunc func() (vlist []Version, err error)

	// Mutex to ensure only one listVersions runs at a time
	//
	// TODO(sdboyer) this is a horrible one-off hack, and must be removed once
	// source managers are refactored to properly serialize and fold-in calls to
	// these methods.
	lvmut sync.Mutex

	// Once-er to control access to syncLocal
	synconce sync.Once

	// The error, if any, that occurred on syncLocal
	syncerr error

	// Whether the cache has the latest info on versions
	cvsync bool
}

func (bs *baseVCSSource) getManifestAndLock(r ProjectRoot, v Version) (Manifest, Lock, error) {
	if err := bs.ensureCacheExistence(); err != nil {
		return nil, nil, err
	}

	rev, err := bs.toRevOrErr(v)
	if err != nil {
		return nil, nil, err
	}

	// Return the info from the cache, if we already have it
	if pi, exists := bs.dc.infos[rev]; exists {
		return pi.Manifest, pi.Lock, nil
	}

	// Cache didn't help; ensure our local is fully up to date.
	do := func() (err error) {
		bs.crepo.mut.Lock()
		// Always prefer a rev, if it's available
		if pv, ok := v.(PairedVersion); ok {
			err = bs.crepo.r.UpdateVersion(pv.Underlying().String())
		} else {
			err = bs.crepo.r.UpdateVersion(v.String())
		}

		bs.crepo.mut.Unlock()
		return
	}

	if err = do(); err != nil {
		// minimize network activity: only force local syncing if we had an err
		err = bs.syncLocal()
		if err != nil {
			return nil, nil, err
		}

		if err = do(); err != nil {
			// TODO(sdboyer) More-er proper-er error
			panic(fmt.Sprintf("canary - why is checkout/whatever failing: %s %s %s", bs.crepo.r.LocalPath(), v.String(), unwrapVcsErr(err)))
		}
	}

	bs.crepo.mut.RLock()
	m, l, err := bs.an.DeriveManifestAndLock(bs.crepo.r.LocalPath(), r)
	// TODO(sdboyer) cache results
	bs.crepo.mut.RUnlock()

	if err == nil {
		if l != nil {
			l = prepLock(l)
		}

		// If m is nil, prepManifest will provide an empty one.
		pi := projectInfo{
			Manifest: prepManifest(m),
			Lock:     l,
		}

		bs.dc.infos[rev] = pi

		return pi.Manifest, pi.Lock, nil
	}

	return nil, nil, unwrapVcsErr(err)
}

// toRevision turns a Version into a Revision, if doing so is possible based on
// the information contained in the version itself, or in the cache maps.
func (dc *sourceMetaCache) toRevision(v Version) Revision {
	switch t := v.(type) {
	case Revision:
		return t
	case PairedVersion:
		return t.Underlying()
	case UnpairedVersion:
		// This will return the empty rev (empty string) if we don't have a
		// record of it. It's up to the caller to decide, for example, if
		// it's appropriate to update the cache.
		return dc.vMap[t]
	default:
		panic(fmt.Sprintf("Unknown version type %T", v))
	}
}

// toUnpaired turns a Version into an UnpairedVersion, if doing so is possible
// based on the information contained in the version itself, or in the cache
// maps.
//
// If the input is a revision and multiple UnpairedVersions are associated with
// it, whatever happens to be the first is returned.
func (dc *sourceMetaCache) toUnpaired(v Version) UnpairedVersion {
	switch t := v.(type) {
	case UnpairedVersion:
		return t
	case PairedVersion:
		return t.Unpair()
	case Revision:
		if upv, has := dc.rMap[t]; has && len(upv) > 0 {
			return upv[0]
		}
		return nil
	default:
		panic(fmt.Sprintf("unknown version type %T", v))
	}
}

func (bs *baseVCSSource) revisionPresentIn(r Revision) (bool, error) {
	// First and fastest path is to check the data cache to see if the rev is
	// present. This could give us false positives, but the cases where that can
	// occur would require a type of cache staleness that seems *exceedingly*
	// unlikely to occur.
	if _, has := bs.dc.infos[r]; has {
		return true, nil
	} else if _, has := bs.dc.rMap[r]; has {
		return true, nil
	}

	err := bs.ensureCacheExistence()
	if err != nil {
		return false, err
	}

	bs.crepo.mut.RLock()
	defer bs.crepo.mut.RUnlock()
	return bs.crepo.r.IsReference(string(r)), nil
}

func (bs *baseVCSSource) ensureCacheExistence() error {
	// Technically, methods could could attempt to return straight from the
	// metadata cache even if the repo cache doesn't exist on disk. But that
	// would allow weird state inconsistencies (cache exists, but no repo...how
	// does that even happen?) that it'd be better to just not allow so that we
	// don't have to think about it elsewhere
	if !bs.checkExistence(existsInCache) {
		if bs.checkExistence(existsUpstream) {
			bs.crepo.mut.Lock()
			if bs.crepo.synced {
				// A second ensure call coming in while the first is completing
				// isn't terribly unlikely, especially for a large repo. In that
				// event, the synced flag will have flipped on by the time we
				// acquire the lock. If it has, there's no need to do this work
				// twice.
				bs.crepo.mut.Unlock()
				return nil
			}

			err := bs.crepo.r.Get()

			if err != nil {
				bs.crepo.mut.Unlock()
				return fmt.Errorf("failed to create repository cache for %s with err:\n%s", bs.crepo.r.Remote(), unwrapVcsErr(err))
			}

			bs.crepo.synced = true
			bs.ex.s |= existsInCache
			bs.ex.f |= existsInCache
			bs.crepo.mut.Unlock()
		} else {
			return fmt.Errorf("project %s does not exist upstream", bs.crepo.r.Remote())
		}
	}

	return nil
}

// checkExistence provides a direct method for querying existence levels of the
// source. It will only perform actual searching (local fs or over the network)
// if no previous attempt at that search has been made.
//
// Note that this may perform read-ish operations on the cache repo, and it
// takes a lock accordingly. This makes it unsafe to call from a segment where
// the cache repo mutex is already write-locked, as deadlock will occur.
func (bs *baseVCSSource) checkExistence(ex sourceExistence) bool {
	if bs.ex.s&ex != ex {
		if ex&existsInVendorRoot != 0 && bs.ex.s&existsInVendorRoot == 0 {
			panic("should now be implemented in bridge")
		}
		if ex&existsInCache != 0 && bs.ex.s&existsInCache == 0 {
			bs.crepo.mut.RLock()
			bs.ex.s |= existsInCache
			if bs.crepo.r.CheckLocal() {
				bs.ex.f |= existsInCache
			}
			bs.crepo.mut.RUnlock()
		}
		if ex&existsUpstream != 0 && bs.ex.s&existsUpstream == 0 {
			bs.crepo.mut.RLock()
			bs.ex.s |= existsUpstream
			if bs.crepo.r.Ping() {
				bs.ex.f |= existsUpstream
			}
			bs.crepo.mut.RUnlock()
		}
	}

	return ex&bs.ex.f == ex
}

// syncLocal ensures the local data we have about the source is fully up to date
// with what's out there over the network.
func (bs *baseVCSSource) syncLocal() error {
	// Ensure we only have one goroutine doing this at a time
	f := func() {
		// First, ensure the local instance exists
		bs.syncerr = bs.ensureCacheExistence()
		if bs.syncerr != nil {
			return
		}

		_, bs.syncerr = bs.lvfunc()
		if bs.syncerr != nil {
			return
		}

		// This case is really just for git repos, where the lvfunc doesn't
		// guarantee that the local repo is synced
		if !bs.crepo.synced {
			bs.crepo.mut.Lock()
			err := bs.crepo.r.Update()
			if err != nil {
				bs.syncerr = fmt.Errorf("failed fetching latest updates with err: %s", unwrapVcsErr(err))
			} else {
				bs.crepo.synced = true
			}
			bs.crepo.mut.Unlock()
		}
	}

	bs.synconce.Do(f)
	return bs.syncerr
}

func (bs *baseVCSSource) listPackages(pr ProjectRoot, v Version) (ptree PackageTree, err error) {
	if err = bs.ensureCacheExistence(); err != nil {
		return
	}

	var r Revision
	if r, err = bs.toRevOrErr(v); err != nil {
		return
	}

	// Return the ptree from the cache, if we already have it
	var exists bool
	if ptree, exists = bs.dc.ptrees[r]; exists {
		return
	}

	// Not in the cache; check out the version and do the analysis
	bs.crepo.mut.Lock()
	// Check out the desired version for analysis
	if r != "" {
		// Always prefer a rev, if it's available
		err = bs.crepo.r.UpdateVersion(string(r))
	} else {
		// If we don't have a rev, ensure the repo is up to date, otherwise we
		// could have a desync issue
		if !bs.crepo.synced {
			err = bs.crepo.r.Update()
			if err != nil {
				err = fmt.Errorf("could not fetch latest updates into repository: %s", unwrapVcsErr(err))
				return
			}
			bs.crepo.synced = true
		}
		err = bs.crepo.r.UpdateVersion(v.String())
	}

	if err == nil {
		ptree, err = ListPackages(bs.crepo.r.LocalPath(), string(pr))
		// TODO(sdboyer) cache errs?
		if err == nil {
			bs.dc.ptrees[r] = ptree
		}
	} else {
		err = unwrapVcsErr(err)
	}
	bs.crepo.mut.Unlock()

	return
}

// toRevOrErr makes all efforts to convert a Version into a rev, including
// updating the cache repo (if needed). It does not guarantee that the returned
// Revision actually exists in the repository (as one of the cheaper methods may
// have had bad data).
func (bs *baseVCSSource) toRevOrErr(v Version) (r Revision, err error) {
	r = bs.dc.toRevision(v)
	if r == "" {
		// Rev can be empty if:
		//  - The cache is unsynced
		//  - A version was passed that used to exist, but no longer does
		//  - A garbage version was passed. (Functionally indistinguishable from
		//  the previous)
		if !bs.cvsync {
			// call the lvfunc to sync the meta cache
			_, err = bs.lvfunc()
			if err != nil {
				return
			}
		}

		r = bs.dc.toRevision(v)
		// If we still don't have a rev, then the version's no good
		if r == "" {
			err = fmt.Errorf("version %s does not exist in source %s", v, bs.crepo.r.Remote())
		}
	}

	return
}

func (bs *baseVCSSource) exportVersionTo(v Version, to string) error {
	if err := bs.ensureCacheExistence(); err != nil {
		return err
	}

	// Only make the parent dir, as the general implementation will balk on
	// trying to write to an empty but existing dir.
	if err := os.MkdirAll(filepath.Dir(to), 0777); err != nil {
		return err
	}

	return bs.crepo.exportVersionTo(v, to)
}
