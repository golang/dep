package gps

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Masterminds/vcs"
)

type source interface {
	checkExistence(projectExistence) bool
	exportVersionTo(Version, string) error
	getManifestAndLock(ProjectRoot, Version) (Manifest, Lock, error)
	listPackages(ProjectRoot, Version) (PackageTree, error)
	listVersions() ([]Version, error)
	revisionPresentIn(ProjectRoot, Revision) (bool, error)
}

// TODO(sdboyer) de-export these fields
type projectDataCache struct {
	Version  string                   `json:"version"` // TODO(sdboyer) use this
	Infos    map[Revision]projectInfo `json:"infos"`
	Packages map[Revision]PackageTree `json:"packages"`
	VMap     map[Version]Revision     `json:"vmap"`
	RMap     map[Revision][]Version   `json:"rmap"`
	// granular mutexes for each map. this has major complexity costs, so we
	// handle elsewhere - but keep these mutexes here as a TODO(sdboyer) to
	// remind that we may want to do this eventually
	//imut, pmut, vmut, rmut sync.RWMutex
}

func newDataCache() *projectDataCache {
	return &projectDataCache{
		Infos:    make(map[Revision]projectInfo),
		Packages: make(map[Revision]PackageTree),
		VMap:     make(map[Version]Revision),
		RMap:     make(map[Revision][]Version),
	}
}

type maybeSource interface {
	try(cachedir string, an ProjectAnalyzer) (source, error)
}

type maybeSources []maybeSource

type maybeGitSource struct {
	n   string
	url *url.URL
}

type gitSource struct {
	baseSource
}

func (m maybeGitSource) try(cachedir string, an ProjectAnalyzer) (source, error) {
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(m.url.String()))
	r, err := vcs.NewGitRepo(m.url.String(), path)
	if err != nil {
		return nil, err
	}

	pm := &gitSource{
		baseSource: baseSource{
			an: an,
			dc: newDataCache(),
			crepo: &repo{
				r:     r,
				rpath: path,
			},
		},
	}

	_, err = pm.listVersions()
	if err != nil {
		return nil, err
		//} else if pm.ex.f&existsUpstream == existsUpstream {
		//return pm, nil
	}

	return pm, nil
}

type baseSource struct { // TODO(sdboyer) rename to baseVCSSource
	// Object for the cache repository
	crepo *repo

	// Indicates the extent to which we have searched for, and verified, the
	// existence of the project/repo.
	ex existence

	// ProjectAnalyzer used to fulfill getManifestAndLock
	an ProjectAnalyzer

	// Whether the cache has the latest info on versions
	cvsync bool

	// The project metadata cache. This is persisted to disk, for reuse across
	// solver runs.
	// TODO(sdboyer) protect with mutex
	dc *projectDataCache
}

func (bs *baseSource) getManifestAndLock(r ProjectRoot, v Version) (Manifest, Lock, error) {
	if err := bs.ensureCacheExistence(); err != nil {
		return nil, nil, err
	}

	if r, exists := bs.dc.VMap[v]; exists {
		if pi, exists := bs.dc.Infos[r]; exists {
			return pi.Manifest, pi.Lock, nil
		}
	}

	bs.crepo.mut.Lock()
	var err error
	if !bs.crepo.synced {
		err = bs.crepo.r.Update()
		if err != nil {
			return nil, nil, fmt.Errorf("Could not fetch latest updates into repository")
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

		// If m is nil, prebsanifest will provide an empty one.
		pi := projectInfo{
			Manifest: prepManifest(m),
			Lock:     l,
		}

		// TODO(sdboyer) this just clobbers all over and ignores the paired/unpaired
		// distinction; serious fix is needed
		if r, exists := bs.dc.VMap[v]; exists {
			bs.dc.Infos[r] = pi
		}

		return pi.Manifest, pi.Lock, nil
	}

	return nil, nil, err
}

func (bs *baseSource) listVersions() (vlist []Version, err error) {
	if !bs.cvsync {
		// This check only guarantees that the upstream exists, not the cache
		bs.ex.s |= existsUpstream
		vpairs, exbits, err := bs.crepo.getCurrentVersionPairs()
		// But it *may* also check the local existence
		bs.ex.s |= exbits
		bs.ex.f |= exbits

		if err != nil {
			// TODO(sdboyer) More-er proper-er error
			return nil, err
		}

		vlist = make([]Version, len(vpairs))
		// mark our cache as synced if we got ExistsUpstream back
		if exbits&existsUpstream == existsUpstream {
			bs.cvsync = true
		}

		// Process the version data into the cache
		// TODO(sdboyer) detect out-of-sync data as we do this?
		for k, v := range vpairs {
			bs.dc.VMap[v] = v.Underlying()
			bs.dc.RMap[v.Underlying()] = append(bs.dc.RMap[v.Underlying()], v)
			vlist[k] = v
		}
	} else {
		vlist = make([]Version, len(bs.dc.VMap))
		k := 0
		// TODO(sdboyer) key type of VMap should be string; recombine here
		//for v, r := range bs.dc.VMap {
		for v := range bs.dc.VMap {
			vlist[k] = v
			k++
		}
	}

	return
}

func (bs *baseSource) ensureCacheExistence() error {
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
				return fmt.Errorf("failed to create repository cache for %s", bs.crepo.r.Remote())
			}
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
func (bs *baseSource) checkExistence(ex projectExistence) bool {
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

func (bs *baseSource) listPackages(pr ProjectRoot, v Version) (ptree PackageTree, err error) {
	if err = bs.ensureCacheExistence(); err != nil {
		return
	}

	// See if we can find it in the cache
	var r Revision
	switch v.(type) {
	case Revision, PairedVersion:
		var ok bool
		if r, ok = v.(Revision); !ok {
			r = v.(PairedVersion).Underlying()
		}

		if ptree, cached := bs.dc.Packages[r]; cached {
			return ptree, nil
		}
	default:
		var has bool
		if r, has = bs.dc.VMap[v]; has {
			if ptree, cached := bs.dc.Packages[r]; cached {
				return ptree, nil
			}
		}
	}

	// TODO(sdboyer) handle the case where we have a version w/out rev, and not in cache

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
				return PackageTree{}, fmt.Errorf("Could not fetch latest updates into repository: %s", err)
			}
			bs.crepo.synced = true
		}
		err = bs.crepo.r.UpdateVersion(v.String())
	}

	ptree, err = listPackages(bs.crepo.r.LocalPath(), string(pr))
	bs.crepo.mut.Unlock()

	// TODO(sdboyer) cache errs?
	if err != nil {
		bs.dc.Packages[r] = ptree
	}

	return
}

func (s *gitSource) exportVersionTo(v Version, to string) error {
	s.crepo.mut.Lock()
	defer s.crepo.mut.Unlock()

	r := s.crepo.r
	// Back up original index
	idx, bak := filepath.Join(r.LocalPath(), ".git", "index"), filepath.Join(r.LocalPath(), ".git", "origindex")
	err := os.Rename(idx, bak)
	if err != nil {
		return err
	}

	// TODO(sdboyer) could have an err here
	defer os.Rename(bak, idx)

	vstr := v.String()
	if rv, ok := v.(PairedVersion); ok {
		vstr = rv.Underlying().String()
	}
	_, err = r.RunFromDir("git", "read-tree", vstr)
	if err != nil {
		return err
	}

	// Ensure we have exactly one trailing slash
	to = strings.TrimSuffix(to, string(os.PathSeparator)) + string(os.PathSeparator)
	// Checkout from our temporary index to the desired target location on disk;
	// now it's git's job to make it fast. Sadly, this approach *does* also
	// write out vendor dirs. There doesn't appear to be a way to make
	// checkout-index respect sparse checkout rules (-a supercedes it);
	// the alternative is using plain checkout, though we have a bunch of
	// housekeeping to do to set up, then tear down, the sparse checkout
	// controls, as well as restore the original index and HEAD.
	_, err = r.RunFromDir("git", "checkout-index", "-a", "--prefix="+to)
	return err
}

func (s *gitSource) listVersions() (vlist []Version, err error) {
	if s.cvsync {
		vlist = make([]Version, len(s.dc.VMap))
		k := 0
		// TODO(sdboyer) key type of VMap should be string; recombine here
		//for v, r := range s.dc.VMap {
		for v := range s.dc.VMap {
			vlist[k] = v
			k++
		}

		return
	}

	r := s.crepo.r
	var out []byte
	c := exec.Command("git", "ls-remote", r.Remote())
	// Ensure no terminal prompting for PWs
	c.Env = mergeEnvLists([]string{"GIT_TERMINAL_PROMPT=0"}, os.Environ())
	out, err = c.CombinedOutput()

	all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	if err != nil || len(all) == 0 {
		// TODO(sdboyer) remove this path? it really just complicates things, for
		// probably not much benefit

		// ls-remote failed, probably due to bad communication or a faulty
		// upstream implementation. So fetch updates, then build the list
		// locally
		s.crepo.mut.Lock()
		err = r.Update()
		s.crepo.mut.Unlock()
		if err != nil {
			// Definitely have a problem, now - bail out
			return
		}

		// Upstream and cache must exist for this to have worked, so add that to
		// searched and found
		s.ex.s |= existsUpstream | existsInCache
		s.ex.f |= existsUpstream | existsInCache
		// Also, local is definitely now synced
		s.crepo.synced = true

		out, err = r.RunFromDir("git", "show-ref", "--dereference")
		if err != nil {
			// TODO(sdboyer) More-er proper-er error
			return
		}

		all = bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		if len(all) == 0 {
			return nil, fmt.Errorf("No versions available for %s (this is weird)", r.Remote())
		}
	}

	// Local cache may not actually exist here, but upstream definitely does
	s.ex.s |= existsUpstream
	s.ex.f |= existsUpstream

	smap := make(map[string]bool)
	uniq := 0
	vlist = make([]Version, len(all)-1) // less 1, because always ignore HEAD
	for _, pair := range all {
		var v PairedVersion
		if string(pair[46:51]) == "heads" {
			v = NewBranch(string(pair[52:])).Is(Revision(pair[:40])).(PairedVersion)
			vlist[uniq] = v
			uniq++
		} else if string(pair[46:50]) == "tags" {
			vstr := string(pair[51:])
			if strings.HasSuffix(vstr, "^{}") {
				// If the suffix is there, then we *know* this is the rev of
				// the underlying commit object that we actually want
				vstr = strings.TrimSuffix(vstr, "^{}")
			} else if smap[vstr] {
				// Already saw the deref'd version of this tag, if one
				// exists, so skip this.
				continue
				// Can only hit this branch if we somehow got the deref'd
				// version first. Which should be impossible, but this
				// covers us in case of weirdness, anyway.
			}
			v = NewVersion(vstr).Is(Revision(pair[:40])).(PairedVersion)
			smap[vstr] = true
			vlist[uniq] = v
			uniq++
		}
	}

	// Trim off excess from the slice
	vlist = vlist[:uniq]

	// Process the version data into the cache
	//
	// reset the rmap and vmap, as they'll be fully repopulated by this
	// TODO(sdboyer) detect out-of-sync pairings as we do this?
	s.dc.VMap = make(map[Version]Revision)
	s.dc.RMap = make(map[Revision][]Version)

	for _, v := range vlist {
		pv := v.(PairedVersion)
		s.dc.VMap[v] = pv.Underlying()
		s.dc.RMap[pv.Underlying()] = append(s.dc.RMap[pv.Underlying()], v)
	}
	// Mark the cache as being in sync with upstream's version list
	s.cvsync = true
	return
}
