package gps

import (
	"sync"

	"github.com/sdboyer/gps/pkgtree"
)

// singleSourceCache provides a method set for storing and retrieving data about
// a single source.
type singleSourceCache interface {
	// Store the manifest and lock information for a given revision, as defined by
	// a particular ProjectAnalyzer.
	setProjectInfo(Revision, ProjectAnalyzer, projectInfo)
	// Get the manifest and lock information for a given revision, as defined by
	// a particular ProjectAnalyzer.
	getProjectInfo(Revision, ProjectAnalyzer) (projectInfo, bool)
	// Store a PackageTree for a given revision.
	setPackageTree(Revision, pkgtree.PackageTree)
	// Get the PackageTree for a given revision.
	getPackageTree(Revision) (pkgtree.PackageTree, bool)
	// Store the mappings between a set of PairedVersions' surface versions
	// their corresponding revisions.
	//
	// If flush is true, the existing list of versions will be purged before
	// writing. Revisions will have their pairings purged, but record of the
	// revision existing will be kept, on the assumption that revisions are
	// immutable and permanent.
	storeVersionMap(versionList []PairedVersion, flush bool)
	// Get the list of unpaired versions corresponding to the given revision.
	getVersionsFor(Revision) ([]UnpairedVersion, bool)
	// Get the revision corresponding to the given unpaired version.
	getRevisionFor(UnpairedVersion) (Revision, bool)
}

type sourceMetaCache struct {
	//Version  string                   // TODO(sdboyer) use this
	infos  map[Revision]projectInfo
	ptrees map[Revision]pkgtree.PackageTree
	vMap   map[UnpairedVersion]Revision
	rMap   map[Revision][]UnpairedVersion
	// TODO(sdboyer) mutexes. actually probably just one, b/c complexity
}

func newMetaCache() *sourceMetaCache {
	return &sourceMetaCache{
		infos:  make(map[Revision]projectInfo),
		ptrees: make(map[Revision]pkgtree.PackageTree),
		vMap:   make(map[UnpairedVersion]Revision),
		rMap:   make(map[Revision][]UnpairedVersion),
	}
}

type singleSourceCacheMemory struct {
	mut    sync.RWMutex // protects all maps
	infos  map[ProjectAnalyzer]map[Revision]projectInfo
	ptrees map[Revision]pkgtree.PackageTree
	vMap   map[UnpairedVersion]Revision
	rMap   map[Revision][]UnpairedVersion
}

func newMemoryCache() singleSourceCache {
	return &singleSourceCacheMemory{
		infos:  make(map[ProjectAnalyzer]map[Revision]projectInfo),
		ptrees: make(map[Revision]pkgtree.PackageTree),
		vMap:   make(map[UnpairedVersion]Revision),
		rMap:   make(map[Revision][]UnpairedVersion),
	}

}
func (c *singleSourceCacheMemory) setProjectInfo(r Revision, an ProjectAnalyzer, pi projectInfo) {
	c.mut.Lock()
	inner, has := c.infos[an]
	if !has {
		inner = make(map[Revision]projectInfo)
		c.infos[an] = inner
	}
	inner[r] = pi
	c.mut.Unlock()
}

func (c *singleSourceCacheMemory) getProjectInfo(r Revision, an ProjectAnalyzer) (projectInfo, bool) {
	c.mut.Lock()
	defer c.mut.Unlock()

	inner, has := c.infos[an]
	if !has {
		return projectInfo{}, false
	}
	pi, has := inner[r]
	return pi, has
}

func (c *singleSourceCacheMemory) setPackageTree(r Revision, ptree pkgtree.PackageTree) {
	c.mut.Lock()
	c.ptrees[r] = ptree
	c.mut.Unlock()
}

func (c *singleSourceCacheMemory) getPackageTree(r Revision) (pkgtree.PackageTree, bool) {
	c.mut.Lock()
	ptree, has := c.ptrees[r]
	c.mut.Unlock()
	return ptree, has
}

func (c *singleSourceCacheMemory) storeVersionMap(versionList []PairedVersion, flush bool) {
	c.mut.Lock()
	if flush {
		for r := range c.rMap {
			c.rMap[r] = nil
		}

		c.vMap = make(map[UnpairedVersion]Revision)
	}

	for _, v := range versionList {
		pv := v.(PairedVersion)
		u, r := pv.Unpair(), pv.Underlying()
		c.vMap[u] = r
		c.rMap[r] = append(c.rMap[r], u)
	}
	c.mut.Unlock()
}

func (c *singleSourceCacheMemory) getVersionsFor(r Revision) ([]UnpairedVersion, bool) {
	c.mut.Lock()
	versionList, has := c.rMap[r]
	c.mut.Unlock()
	return versionList, has
}

func (c *singleSourceCacheMemory) getRevisionFor(uv UnpairedVersion) (Revision, bool) {
	c.mut.Lock()
	r, has := c.vMap[uv]
	c.mut.Unlock()
	return r, has
}
