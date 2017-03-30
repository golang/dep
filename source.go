package gps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sdboyer/gps/pkgtree"
)

// sourceExistence values represent the extent to which a project "exists."
// TODO remove
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

type sourceState int32

const (
	sourceIsSetUp sourceState = 1 << iota
	sourceExistsUpstream
	sourceExistsLocally
	sourceHasLatestVersionList
	sourceHasLatestLocally
)

type srcReturnChans struct {
	ret chan *sourceGateway
	err chan error
}

func (rc srcReturnChans) awaitReturn() (sg *sourceGateway, err error) {
	select {
	case sg = <-rc.ret:
	case err = <-rc.err:
	}
	return
}

type sourceCoordinator struct {
	callMgr   *callManager
	srcmut    sync.RWMutex // guards srcs and nameToURL maps
	srcs      map[string]*sourceGateway
	nameToURL map[string]string
	psrcmut   sync.Mutex // guards protoSrcs map
	protoSrcs map[string][]srcReturnChans
	deducer   *deductionCoordinator
	cachedir  string
}

func newSourceCoordinator(cm *callManager, deducer *deductionCoordinator, cachedir string) *sourceCoordinator {
	return &sourceCoordinator{
		callMgr:   cm,
		deducer:   deducer,
		cachedir:  cachedir,
		srcs:      make(map[string]*sourceGateway),
		nameToURL: make(map[string]string),
		protoSrcs: make(map[string][]srcReturnChans),
	}
}

func (sc *sourceCoordinator) getSourceGatewayFor(ctx context.Context, id ProjectIdentifier) (*sourceGateway, error) {
	if sc.callMgr.getLifetimeContext().Err() != nil {
		return nil, errors.New("sourceCoordinator has been terminated")
	}

	normalizedName := id.normalizedSource()

	sc.srcmut.RLock()
	if url, has := sc.nameToURL[normalizedName]; has {
		srcGate, has := sc.srcs[url]
		sc.srcmut.RUnlock()
		if has {
			return srcGate, nil
		}
		panic(fmt.Sprintf("%q was URL for %q in nameToURL, but no corresponding srcGate in srcs map", url, normalizedName))
	}
	sc.srcmut.RUnlock()

	// No gateway exists for this path yet; set up a proto, being careful to fold
	// together simultaneous attempts on the same path.
	rc := srcReturnChans{
		ret: make(chan *sourceGateway),
		err: make(chan error),
	}

	// The rest of the work needs its own goroutine, the results of which will
	// be re-joined to this call via the return chans.
	go sc.setUpSourceGateway(ctx, normalizedName, rc)
	return rc.awaitReturn()
}

// Not intended to be called externally - call getSourceGatewayFor instead.
func (sc *sourceCoordinator) setUpSourceGateway(ctx context.Context, normalizedName string, rc srcReturnChans) {
	sc.psrcmut.Lock()
	if chans, has := sc.protoSrcs[normalizedName]; has {
		// Another goroutine is already working on this normalizedName. Fold
		// in with that work by attaching our return channels to the list.
		sc.protoSrcs[normalizedName] = append(chans, rc)
		sc.psrcmut.Unlock()
		return
	}

	sc.protoSrcs[normalizedName] = []srcReturnChans{rc}
	sc.psrcmut.Unlock()

	doReturn := func(sg *sourceGateway, err error) {
		sc.psrcmut.Lock()
		if sg != nil {
			for _, rc := range sc.protoSrcs[normalizedName] {
				rc.ret <- sg
			}
		} else if err != nil {
			for _, rc := range sc.protoSrcs[normalizedName] {
				rc.err <- err
			}
		} else {
			panic("sg and err both nil")
		}

		delete(sc.protoSrcs, normalizedName)
		sc.psrcmut.Unlock()
	}

	pd, err := sc.deducer.deduceRootPath(ctx, normalizedName)
	if err != nil {
		// As in the deducer, don't cache errors so that externally-driven retry
		// strategies can be constructed.
		doReturn(nil, err)
		return
	}

	// It'd be quite the feat - but not impossible - for a gateway
	// corresponding to this normalizedName to have slid into the main
	// sources map after the initial unlock, but before this goroutine got
	// scheduled. Guard against that by checking the main sources map again
	// and bailing out if we find an entry.
	var srcGate *sourceGateway
	sc.srcmut.RLock()
	if url, has := sc.nameToURL[normalizedName]; has {
		if srcGate, has := sc.srcs[url]; has {
			sc.srcmut.RUnlock()
			doReturn(srcGate, nil)
			return
		}
		panic(fmt.Sprintf("%q was URL for %q in nameToURL, but no corresponding srcGate in srcs map", url, normalizedName))
	}
	sc.srcmut.RUnlock()

	srcGate = newSourceGateway(pd.mb, sc.callMgr, sc.cachedir)

	// The normalized name is usually different from the source URL- e.g.
	// github.com/sdboyer/gps vs. https://github.com/sdboyer/gps. But it's
	// possible to arrive here with a full URL as the normalized name - and
	// both paths *must* lead to the same sourceGateway instance in order to
	// ensure disk access is correctly managed.
	//
	// Therefore, we now must query the sourceGateway to get the actual
	// sourceURL it's operating on, and ensure it's *also* registered at
	// that path in the map. This will cause it to actually initiate the
	// maybeSource.try() behavior in order to settle on a URL.
	url, err := srcGate.sourceURL(ctx)
	if err != nil {
		doReturn(nil, err)
		return
	}

	// We know we have a working srcGateway at this point, and need to
	// integrate it back into the main map.
	sc.srcmut.Lock()
	defer sc.srcmut.Unlock()
	// Record the name -> URL mapping, even if it's a self-mapping.
	sc.nameToURL[normalizedName] = url

	if sa, has := sc.srcs[url]; has {
		// URL already had an entry in the main map; use that as the result.
		doReturn(sa, nil)
		return
	}

	sc.srcs[url] = srcGate
	doReturn(srcGate, nil)
}

// sourceGateways manage all incoming calls for data from sources, serializing
// and caching them as needed.
type sourceGateway struct {
	cachedir string
	maybe    maybeSource
	srcState sourceState
	src      source
	cache    singleSourceCache
	mu       sync.Mutex // global lock, serializes all behaviors
	callMgr  *callManager
}

func newSourceGateway(maybe maybeSource, callMgr *callManager, cachedir string) *sourceGateway {
	sg := &sourceGateway{
		maybe:    maybe,
		cachedir: cachedir,
		callMgr:  callMgr,
	}
	sg.cache = sg.createSingleSourceCache()

	return sg
}

func (sg *sourceGateway) syncLocal(ctx context.Context) error {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	_, err := sg.require(ctx, sourceIsSetUp|sourceHasLatestLocally)
	return err
}

func (sg *sourceGateway) checkExistence(ctx context.Context, ex sourceExistence) bool {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	if ex&existsUpstream != 0 {
		// TODO(sdboyer) these constants really aren't conceptual siblings in the
		// way they should be
		_, err := sg.require(ctx, sourceIsSetUp|sourceExistsUpstream)
		if err != nil {
			return false
		}
	}
	if ex&existsInCache != 0 {
		_, err := sg.require(ctx, sourceIsSetUp|sourceExistsLocally)
		if err != nil {
			return false
		}
	}

	return true
}

func (sg *sourceGateway) exportVersionTo(ctx context.Context, v Version, to string) error {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	_, err := sg.require(ctx, sourceIsSetUp|sourceExistsLocally)
	if err != nil {
		return err
	}

	r, err := sg.convertToRevision(ctx, v)
	if err != nil {
		return err
	}

	return sg.src.exportRevisionTo(r, to)
}

func (sg *sourceGateway) getManifestAndLock(ctx context.Context, pr ProjectRoot, v Version, an ProjectAnalyzer) (Manifest, Lock, error) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	r, err := sg.convertToRevision(ctx, v)
	if err != nil {
		return nil, nil, err
	}

	pi, has := sg.cache.getProjectInfo(r, an)
	if has {
		return pi.Manifest, pi.Lock, nil
	}

	_, err = sg.require(ctx, sourceIsSetUp|sourceExistsLocally)
	if err != nil {
		return nil, nil, err
	}

	m, l, err := sg.src.getManifestAndLock(pr, r, an)
	if err != nil {
		return nil, nil, err
	}

	sg.cache.setProjectInfo(r, an, projectInfo{Manifest: m, Lock: l})
	return m, l, nil
}

// FIXME ProjectRoot input either needs to parameterize the cache, or be
// incorporated on the fly on egress...?
func (sg *sourceGateway) listPackages(ctx context.Context, pr ProjectRoot, v Version) (pkgtree.PackageTree, error) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	r, err := sg.convertToRevision(ctx, v)
	if err != nil {
		return pkgtree.PackageTree{}, err
	}

	ptree, has := sg.cache.getPackageTree(r)
	if has {
		return ptree, nil
	}

	_, err = sg.require(ctx, sourceIsSetUp|sourceExistsLocally)
	if err != nil {
		return pkgtree.PackageTree{}, err
	}

	ptree, err = sg.src.listPackages(pr, r)
	if err != nil {
		return pkgtree.PackageTree{}, err
	}

	sg.cache.setPackageTree(r, ptree)
	return ptree, nil
}

func (sg *sourceGateway) convertToRevision(ctx context.Context, v Version) (Revision, error) {
	// When looking up by Version, there are four states that may have
	// differing opinions about version->revision mappings:
	//
	//   1. The upstream source/repo (canonical)
	//   2. The local source/repo
	//   3. The local cache
	//   4. The input (params to this method)
	//
	// If the input differs from any of the above, it's likely because some lock
	// got written somewhere with a version/rev pair that has since changed or
	// been removed. But correct operation dictates that such a mis-mapping be
	// respected; if the mis-mapping is to be corrected, it has to be done
	// intentionally by the caller, not automatically here.
	r, has := sg.cache.toRevision(v)
	if has {
		return r, nil
	}

	if sg.srcState&sourceHasLatestVersionList != 0 {
		// We have the latest version list already and didn't get a match, so
		// this is definitely a failure case.
		return "", fmt.Errorf("version %q does not exist in source", v)
	}

	// The version list is out of date; it's possible this version might
	// show up after loading it.
	_, err := sg.require(ctx, sourceIsSetUp|sourceHasLatestVersionList)
	if err != nil {
		return "", err
	}

	r, has = sg.cache.toRevision(v)
	if !has {
		return "", fmt.Errorf("version %q does not exist in source", v)
	}

	return r, nil
}

func (sg *sourceGateway) listVersions(ctx context.Context) ([]PairedVersion, error) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	// TODO(sdboyer) The problem here is that sourceExistsUpstream may not be
	// sufficient (e.g. bzr, hg), but we don't want to force local b/c git
	// doesn't need it
	_, err := sg.require(ctx, sourceIsSetUp|sourceExistsUpstream|sourceHasLatestVersionList)
	if err != nil {
		return nil, err
	}

	return sg.cache.getAllVersions(), nil
}

func (sg *sourceGateway) revisionPresentIn(ctx context.Context, r Revision) (bool, error) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	_, err := sg.require(ctx, sourceIsSetUp|sourceExistsLocally)
	if err != nil {
		return false, err
	}

	if _, exists := sg.cache.getVersionsFor(r); exists {
		return true, nil
	}

	present, err := sg.src.revisionPresentIn(r)
	if err == nil && present {
		sg.cache.markRevisionExists(r)
	}
	return present, err
}

func (sg *sourceGateway) sourceURL(ctx context.Context) (string, error) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	_, err := sg.require(ctx, sourceIsSetUp)
	if err != nil {
		return "", err
	}

	return sg.src.upstreamURL(), nil
}

// createSingleSourceCache creates a singleSourceCache instance for use by
// the encapsulated source.
func (sg *sourceGateway) createSingleSourceCache() singleSourceCache {
	// TODO(sdboyer) when persistent caching is ready, just drop in the creation
	// of a source-specific handle here
	return newMemoryCache()
}

func (sg *sourceGateway) require(ctx context.Context, wanted sourceState) (errState sourceState, err error) {
	todo := (^sg.srcState) & wanted
	var flag sourceState = 1

	for todo != 0 {
		if todo&flag != 0 {
			// Assign the currently visited bit to errState so that we can
			// return easily later.
			//
			// Also set up addlState so that individual ops can easily attach
			// more states that were incidentally satisfied by the op.
			errState = flag
			var addlState sourceState

			switch flag {
			case sourceIsSetUp:
				sg.src, addlState, err = sg.maybe.try(ctx, sg.cachedir, sg.cache)
			case sourceExistsUpstream:
				if !sg.src.existsUpstream(ctx) {
					err = fmt.Errorf("%s does not exist upstream", sg.src.upstreamURL())
				}
			case sourceExistsLocally:
				if !sg.src.existsLocally(ctx) {
					err = sg.src.initLocal()
					if err == nil {
						addlState |= sourceHasLatestLocally
					} else {
						err = fmt.Errorf("%s does not exist in the local cache and fetching failed: %s", sg.src.upstreamURL(), err)
					}
				}
			case sourceHasLatestVersionList:
				var pvl []PairedVersion
				pvl, err = sg.src.listVersions()
				if err != nil {
					sg.cache.storeVersionMap(pvl, true)
				}
			case sourceHasLatestLocally:
				err = sg.src.updateLocal()
			}

			if err != nil {
				return
			}

			checked := flag | addlState
			sg.srcState |= checked
			todo &= ^checked
		}

		flag <<= 1
	}

	return 0, nil
}

// source is an abstraction around the different underlying types (git, bzr, hg,
// svn, maybe raw on-disk code, and maybe eventually a registry) that can
// provide versioned project source trees.
type source interface {
	existsLocally(context.Context) bool
	existsUpstream(context.Context) bool
	upstreamURL() string
	initLocal() error
	updateLocal() error
	listVersions() ([]PairedVersion, error)
	getManifestAndLock(ProjectRoot, Revision, ProjectAnalyzer) (Manifest, Lock, error)
	listPackages(ProjectRoot, Revision) (pkgtree.PackageTree, error)
	revisionPresentIn(Revision) (bool, error)
	exportRevisionTo(Revision, string) error
}

// projectInfo holds manifest and lock
type projectInfo struct {
	Manifest
	Lock
}

type baseVCSSource struct {
	// Object for the cache repository
	crepo *repo

	// The project metadata cache. This is (or is intended to be) persisted to
	// disk, for reuse across solver runs.
	dc singleSourceCache
}

func (bs *baseVCSSource) existsLocally(ctx context.Context) bool {
	return bs.crepo.r.CheckLocal()
}

func (bs *baseVCSSource) existsUpstream(ctx context.Context) bool {
	return bs.crepo.r.Ping()
}

func (bs *baseVCSSource) upstreamURL() string {
	return bs.crepo.r.Remote()
}

func (bs *baseVCSSource) getManifestAndLock(pr ProjectRoot, r Revision, an ProjectAnalyzer) (Manifest, Lock, error) {
	bs.crepo.mut.Lock()
	err := bs.crepo.r.UpdateVersion(r.String())
	m, l, err := an.DeriveManifestAndLock(bs.crepo.r.LocalPath(), pr)
	bs.crepo.mut.Unlock()

	if err != nil {
		return nil, nil, unwrapVcsErr(err)
	}

	if l != nil && l != Lock(nil) {
		l = prepLock(l)
	}

	return prepManifest(m), l, nil
}

func (bs *baseVCSSource) revisionPresentIn(r Revision) (bool, error) {
	bs.crepo.mut.RLock()
	defer bs.crepo.mut.RUnlock()
	return bs.crepo.r.IsReference(string(r)), nil
}

// initLocal clones/checks out the upstream repository to disk for the first
// time.
func (bs *baseVCSSource) initLocal() error {
	bs.crepo.mut.Lock()
	err := bs.crepo.r.Get()
	bs.crepo.mut.Unlock()
	if err != nil {
		return unwrapVcsErr(err)
	}
	return nil
}

// updateLocal ensures the local data we have about the source is fully up to date
// with what's out there over the network.
func (bs *baseVCSSource) updateLocal() error {
	bs.crepo.mut.Lock()
	err := bs.crepo.r.Update()
	bs.crepo.mut.Unlock()
	if err != nil {
		return unwrapVcsErr(err)
	}
	return nil
}

func (bs *baseVCSSource) listPackages(pr ProjectRoot, r Revision) (ptree pkgtree.PackageTree, err error) {
	bs.crepo.mut.Lock()
	// Check out the desired version for analysis
	err = bs.crepo.r.UpdateVersion(string(r))

	if err != nil {
		err = unwrapVcsErr(err)
	} else {
		ptree, err = pkgtree.ListPackages(bs.crepo.r.LocalPath(), string(pr))
	}
	bs.crepo.mut.Unlock()

	return
}

func (bs *baseVCSSource) exportRevisionTo(r Revision, to string) error {
	// Only make the parent dir, as the general implementation will balk on
	// trying to write to an empty but existing dir.
	if err := os.MkdirAll(filepath.Dir(to), 0777); err != nil {
		return err
	}

	return bs.crepo.exportVersionTo(r, to)
}
