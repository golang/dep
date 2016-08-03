package gps

import (
	"fmt"
	"go/build"
)

type projectManager struct {
	// The upstream URL from which the project is sourced.
	n string

	// build.Context to use in any analysis, and to pass to the analyzer
	ctx build.Context

	// Object for the cache repository
	crepo *repo

	// Indicates the extent to which we have searched for, and verified, the
	// existence of the project/repo.
	ex existence

	// Analyzer, injected by way of the SourceManager and originally from the
	// sm's creator
	an ProjectAnalyzer

	// Whether the cache has the latest info on versions
	cvsync bool

	// The project metadata cache. This is persisted to disk, for reuse across
	// solver runs.
	// TODO(sdboyer) protect with mutex
	dc *sourceMetaCache
}

type existence struct {
	// The existence levels for which a search/check has been performed
	s sourceExistence

	// The existence levels verified to be present through searching
	f sourceExistence
}

// projectInfo holds manifest and lock
type projectInfo struct {
	Manifest
	Lock
}

func (pm *projectManager) GetManifestAndLock(r ProjectRoot, v Version) (Manifest, Lock, error) {
	if err := pm.ensureCacheExistence(); err != nil {
		return nil, nil, err
	}

	rev, err := pm.toRevOrErr(v)
	if err != nil {
		return nil, nil, err
	}

	// Return the info from the cache, if we already have it
	if pi, exists := pm.dc.infos[rev]; exists {
		return pi.Manifest, pi.Lock, nil
	}

	pm.crepo.mut.Lock()
	if !pm.crepo.synced {
		err = pm.crepo.r.Update()
		if err != nil {
			return nil, nil, fmt.Errorf("could not fetch latest updates into repository")
		}
		pm.crepo.synced = true
	}

	// Always prefer a rev, if it's available
	if pv, ok := v.(PairedVersion); ok {
		err = pm.crepo.r.UpdateVersion(pv.Underlying().String())
	} else {
		err = pm.crepo.r.UpdateVersion(v.String())
	}
	pm.crepo.mut.Unlock()
	if err != nil {
		// TODO(sdboyer) More-er proper-er error
		panic(fmt.Sprintf("canary - why is checkout/whatever failing: %s %s %s", pm.n, v.String(), err))
	}

	pm.crepo.mut.RLock()
	m, l, err := pm.an.DeriveManifestAndLock(pm.crepo.rpath, r)
	// TODO(sdboyer) cache results
	pm.crepo.mut.RUnlock()

	if err == nil {
		if l != nil {
			l = prepLock(l)
		}

		// If m is nil, prepManifest will provide an empty one.
		pi := projectInfo{
			Manifest: prepManifest(m),
			Lock:     l,
		}

		// TODO(sdboyer) this just clobbers all over and ignores the paired/unpaired
		// distinction; serious fix is needed
		pm.dc.infos[rev] = pi

		return pi.Manifest, pi.Lock, nil
	}

	return nil, nil, err
}

func (pm *projectManager) ListPackages(pr ProjectRoot, v Version) (ptree PackageTree, err error) {
	if err = pm.ensureCacheExistence(); err != nil {
		return
	}

	var r Revision
	if r, err = pm.toRevOrErr(v); err != nil {
		return
	}

	// Return the ptree from the cache, if we already have it
	var exists bool
	if ptree, exists = pm.dc.ptrees[r]; exists {
		return
	}

	// Not in the cache; check out the version and do the analysis
	pm.crepo.mut.Lock()
	// Check out the desired version for analysis
	if r != "" {
		// Always prefer a rev, if it's available
		err = pm.crepo.r.UpdateVersion(string(r))
	} else {
		// If we don't have a rev, ensure the repo is up to date, otherwise we
		// could have a desync issue
		if !pm.crepo.synced {
			err = pm.crepo.r.Update()
			if err != nil {
				return PackageTree{}, fmt.Errorf("could not fetch latest updates into repository: %s", err)
			}
			pm.crepo.synced = true
		}
		err = pm.crepo.r.UpdateVersion(v.String())
	}

	ptree, err = listPackages(pm.crepo.rpath, string(pr))
	pm.crepo.mut.Unlock()

	// TODO(sdboyer) cache errs?
	if err != nil {
		pm.dc.ptrees[r] = ptree
	}

	return
}

func (pm *projectManager) ensureCacheExistence() error {
	// Technically, methods could could attempt to return straight from the
	// metadata cache even if the repo cache doesn't exist on disk. But that
	// would allow weird state inconsistencies (cache exists, but no repo...how
	// does that even happen?) that it'd be better to just not allow so that we
	// don't have to think about it elsewhere
	if !pm.CheckExistence(existsInCache) {
		if pm.CheckExistence(existsUpstream) {
			pm.crepo.mut.Lock()
			err := pm.crepo.r.Get()
			pm.crepo.mut.Unlock()

			if err != nil {
				return fmt.Errorf("failed to create repository cache for %s", pm.n)
			}
			pm.ex.s |= existsInCache
			pm.ex.f |= existsInCache
		} else {
			return fmt.Errorf("project %s does not exist upstream", pm.n)
		}
	}

	return nil
}

func (pm *projectManager) ListVersions() (vlist []Version, err error) {
	if !pm.cvsync {
		// This check only guarantees that the upstream exists, not the cache
		pm.ex.s |= existsUpstream
		vpairs, exbits, err := pm.crepo.getCurrentVersionPairs()
		// But it *may* also check the local existence
		pm.ex.s |= exbits
		pm.ex.f |= exbits

		if err != nil {
			// TODO(sdboyer) More-er proper-er error
			return nil, err
		}

		vlist = make([]Version, len(vpairs))
		// mark our cache as synced if we got ExistsUpstream back
		if exbits&existsUpstream == existsUpstream {
			pm.cvsync = true
		}

		// Process the version data into the cache
		// TODO(sdboyer) detect out-of-sync data as we do this?
		for k, v := range vpairs {
			u, r := v.Unpair(), v.Underlying()
			pm.dc.vMap[u] = r
			pm.dc.rMap[r] = append(pm.dc.rMap[r], u)
			vlist[k] = v
		}
	} else {
		vlist = make([]Version, len(pm.dc.vMap))
		k := 0
		for v, r := range pm.dc.vMap {
			vlist[k] = v.Is(r)
			k++
		}
	}

	return
}

// toRevOrErr makes all efforts to convert a Version into a rev, including
// updating the cache repo (if needed). It does not guarantee that the returned
// Revision actually exists in the repository (as one of the cheaper methods may
// have had bad data).
func (pm *projectManager) toRevOrErr(v Version) (r Revision, err error) {
	r = pm.dc.toRevision(v)
	if r == "" {
		// Rev can be empty if:
		//  - The cache is unsynced
		//  - A version was passed that used to exist, but no longer does
		//  - A garbage version was passed. (Functionally indistinguishable from
		//  the previous)
		if !pm.cvsync {
			_, err = pm.ListVersions()
			if err != nil {
				return
			}
		}

		r = pm.dc.toRevision(v)
		// If we still don't have a rev, then the version's no good
		if r == "" {
			err = fmt.Errorf("version %s does not exist in source %s", v, pm.crepo.r.Remote())
		}
	}

	return
}

func (pm *projectManager) RevisionPresentIn(pr ProjectRoot, r Revision) (bool, error) {
	// First and fastest path is to check the data cache to see if the rev is
	// present. This could give us false positives, but the cases where that can
	// occur would require a type of cache staleness that seems *exceedingly*
	// unlikely to occur.
	if _, has := pm.dc.infos[r]; has {
		return true, nil
	} else if _, has := pm.dc.rMap[r]; has {
		return true, nil
	}

	// For now at least, just run GetInfoAt(); it basically accomplishes the
	// same thing.
	if _, _, err := pm.GetManifestAndLock(pr, r); err != nil {
		return false, err
	}
	return true, nil
}

// CheckExistence provides a direct method for querying existence levels of the
// project. It will only perform actual searching (local fs or over the network)
// if no previous attempt at that search has been made.
//
// Note that this may perform read-ish operations on the cache repo, and it
// takes a lock accordingly. Deadlock may result from calling it during a
// segment where the cache repo mutex is already write-locked.
func (pm *projectManager) CheckExistence(ex sourceExistence) bool {
	if pm.ex.s&ex != ex {
		if ex&existsInVendorRoot != 0 && pm.ex.s&existsInVendorRoot == 0 {
			panic("should now be implemented in bridge")
		}
		if ex&existsInCache != 0 && pm.ex.s&existsInCache == 0 {
			pm.crepo.mut.RLock()
			pm.ex.s |= existsInCache
			if pm.crepo.r.CheckLocal() {
				pm.ex.f |= existsInCache
			}
			pm.crepo.mut.RUnlock()
		}
		if ex&existsUpstream != 0 && pm.ex.s&existsUpstream == 0 {
			pm.crepo.mut.RLock()
			pm.ex.s |= existsUpstream
			if pm.crepo.r.Ping() {
				pm.ex.f |= existsUpstream
			}
			pm.crepo.mut.RUnlock()
		}
	}

	return ex&pm.ex.f == ex
}

func (pm *projectManager) ExportVersionTo(v Version, to string) error {
	return pm.crepo.exportVersionTo(v, to)
}
