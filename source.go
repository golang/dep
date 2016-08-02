package gps

import (
	"fmt"
	"net/url"
	"path/filepath"

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

type projectDataCache struct {
	Version  string                   `json:"version"` // TODO(sdboyer) use this
	Infos    map[Revision]projectInfo `json:"infos"`
	Packages map[Revision]PackageTree `json:"packages"`
	VMap     map[Version]Revision     `json:"vmap"`
	RMap     map[Revision][]Version   `json:"rmap"`
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

func (s maybeGitSource) try(cachedir string, an ProjectAnalyzer) (source, error) {
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(s.url.String()))
	pm := &gitSource{
		baseSource: baseSource{
			an: an,
			dc: newDataCache(),
			crepo: &repo{
				r:     vcs.NewGitRepo(path, s.url.String()),
				rpath: path,
			},
		},
	}

	_, err := pm.ListVersions()
	if err != nil {
		return nil, err
		//} else if pm.ex.f&existsUpstream == existsUpstream {
		//return pm, nil
	}

	return pm, nil
}

type baseSource struct {
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
		panic(fmt.Sprintf("canary - why is checkout/whatever failing: %s %s %s", bs.n, v.String(), err))
	}

	bs.crepo.mut.RLock()
	m, l, err := bs.an.DeriveManifestAndLock(bs.crepo.rpath, r)
	// TODO(sdboyer) cache results
	bs.crepo.mut.RUnlock()

	if err == nil {
		if l != nil {
			l = prepLock(l)
		}

		// If m is nil, prebsanifest will provide an empty one.
		pi := projectInfo{
			Manifest: prebsanifest(m),
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

type gitSource struct {
	bs baseSource
}
