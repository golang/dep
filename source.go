package gps

import (
	"fmt"
	"sync"
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

	// lock to serialize access to syncLocal
	synclock sync.Mutex

	// Globalish flag indicating whether a "full" sync has been performed. Also
	// used as a one-way gate to ensure that the full syncing routine is never
	// run more than once on a given source instance.
	allsync bool

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

	bs.crepo.mut.Lock()
	if !bs.crepo.synced {
		err = bs.crepo.r.Update()
		if err != nil {
			return nil, nil, fmt.Errorf("could not fetch latest updates into repository")
		}
		bs.crepo.synced = true
	}

	// Always prefer a rev, if it's available
	if pv, ok := v.(PairedVersion); ok {
		err = bs.crepo.r.UpdateVersion(pv.Underlying().String())
	} else {
		err = bs.crepo.r.UpdateVersion(v.String())
	}
	bs.crepo.mut.Unlock()

	if err != nil {
		// TODO(sdboyer) More-er proper-er error
		panic(fmt.Sprintf("canary - why is checkout/whatever failing: %s %s %s", bs.crepo.r.LocalPath(), v.String(), err))
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

	return nil, nil, err
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
			err := bs.crepo.r.Get()
			bs.crepo.mut.Unlock()

			if err != nil {
				return fmt.Errorf("failed to create repository cache for %s with err:\n%s", bs.crepo.r.Remote(), err)
			}
			bs.crepo.synced = true
			bs.ex.s |= existsInCache
			bs.ex.f |= existsInCache
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
	bs.synclock.Lock()
	defer bs.synclock.Unlock()

	// ...and that we only ever do it once
	if bs.allsync {
		// Return the stored err, if any
		return bs.syncerr
	}

	bs.allsync = true
	// First, ensure the local instance exists
	bs.syncerr = bs.ensureCacheExistence()
	if bs.syncerr != nil {
		return bs.syncerr
	}

	_, bs.syncerr = bs.lvfunc()
	if bs.syncerr != nil {
		return bs.syncerr
	}

	// This case is really just for git repos, where the lvfunc doesn't
	// guarantee that the local repo is synced
	if !bs.crepo.synced {
		bs.syncerr = bs.crepo.r.Update()
		if bs.syncerr != nil {
			return bs.syncerr
		}
		bs.crepo.synced = true
	}

	return nil
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
				return PackageTree{}, fmt.Errorf("could not fetch latest updates into repository: %s", err)
			}
			bs.crepo.synced = true
		}
		err = bs.crepo.r.UpdateVersion(v.String())
	}

	ptree, err = listPackages(bs.crepo.r.LocalPath(), string(pr))
	bs.crepo.mut.Unlock()

	// TODO(sdboyer) cache errs?
	if err != nil {
		bs.dc.ptrees[r] = ptree
	}

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
	return bs.crepo.exportVersionTo(v, to)
}
