package gps

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/Masterminds/vcs"
)

type maybeSource interface {
	try(cachedir string, an ProjectAnalyzer) (source, error)
}

type maybeSources []maybeSource

func (mbs maybeSources) try(cachedir string, an ProjectAnalyzer) (source, error) {
	var e sourceFailures
	for _, mb := range mbs {
		src, err := mb.try(cachedir, an)
		if err == nil {
			return src, nil
		}
		e = append(e, err)
	}
	return nil, e
}

type sourceFailures []error

func (sf sourceFailures) Error() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "No valid source could be created:\n")
	for _, e := range sf {
		fmt.Fprintf(&buf, "\t%s", e.Error())
	}

	return buf.String()
}

type maybeGitSource struct {
	url *url.URL
}

func (m maybeGitSource) try(cachedir string, an ProjectAnalyzer) (source, error) {
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(m.url.String()))
	r, err := vcs.NewGitRepo(m.url.String(), path)
	if err != nil {
		return nil, err
	}

	src := &gitSource{
		baseVCSSource: baseVCSSource{
			an: an,
			dc: newMetaCache(),
			crepo: &repo{
				r:     r,
				rpath: path,
			},
		},
	}

	_, err = src.listVersions()
	if err != nil {
		return nil, err
		//} else if pm.ex.f&existsUpstream == existsUpstream {
		//return pm, nil
	}

	return src, nil
}

type maybeBzrSource struct {
	url *url.URL
}

func (m maybeBzrSource) try(cachedir string, an ProjectAnalyzer) (source, error) {
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(m.url.String()))
	r, err := vcs.NewBzrRepo(m.url.String(), path)
	if err != nil {
		return nil, err
	}
	if !r.Ping() {
		return nil, fmt.Errorf("Remote repository at %s does not exist, or is inaccessible", m.url.String())
	}

	return &bzrSource{
		baseVCSSource: baseVCSSource{
			an: an,
			dc: newMetaCache(),
			crepo: &repo{
				r:     r,
				rpath: path,
			},
		},
	}, nil
}

type maybeHgSource struct {
	url *url.URL
}

func (m maybeHgSource) try(cachedir string, an ProjectAnalyzer) (source, error) {
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(m.url.String()))
	r, err := vcs.NewHgRepo(m.url.String(), path)
	if err != nil {
		return nil, err
	}
	if !r.Ping() {
		return nil, fmt.Errorf("Remote repository at %s does not exist, or is inaccessible", m.url.String())
	}

	return &hgSource{
		baseVCSSource: baseVCSSource{
			an: an,
			dc: newMetaCache(),
			crepo: &repo{
				r:     r,
				rpath: path,
			},
		},
	}, nil
}
