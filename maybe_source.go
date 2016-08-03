package gps

import (
	"net/url"
	"path/filepath"

	"github.com/Masterminds/vcs"
)

type maybeSource interface {
	try(cachedir string, an ProjectAnalyzer) (source, error)
}

type maybeSources []maybeSource

type maybeGitSource struct {
	n   string
	url *url.URL
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
