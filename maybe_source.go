package gps

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/Masterminds/vcs"
)

type maybeSource interface {
	try(cachedir string, an ProjectAnalyzer) (source, string, error)
}

type maybeSources []maybeSource

func (mbs maybeSources) try(cachedir string, an ProjectAnalyzer) (source, string, error) {
	var e sourceFailures
	for _, mb := range mbs {
		src, ident, err := mb.try(cachedir, an)
		if err == nil {
			return src, ident, nil
		}
		e = append(e, sourceSetupFailure{
			ident: ident,
			err:   err,
		})
	}
	return nil, "", e
}

type sourceSetupFailure struct {
	ident string
	err   error
}

func (e sourceSetupFailure) Error() string {
	return fmt.Sprintf("failed to set up %q, error %s", e.ident, e.err.Error())
}

type sourceFailures []sourceSetupFailure

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

func (m maybeGitSource) try(cachedir string, an ProjectAnalyzer) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewGitRepo(ustr, path)
	if err != nil {
		return nil, "", err
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
		return nil, "", err
		//} else if pm.ex.f&existsUpstream == existsUpstream {
		//return pm, nil
	}

	return src, ustr, nil
}

type maybeBzrSource struct {
	url *url.URL
}

func (m maybeBzrSource) try(cachedir string, an ProjectAnalyzer) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewBzrRepo(ustr, path)
	if err != nil {
		return nil, "", err
	}
	if !r.Ping() {
		return nil, "", fmt.Errorf("Remote repository at %s does not exist, or is inaccessible", ustr)
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
	}, ustr, nil
}

type maybeHgSource struct {
	url *url.URL
}

func (m maybeHgSource) try(cachedir string, an ProjectAnalyzer) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewBzrRepo(ustr, path)
	if err != nil {
		return nil, "", err
	}
	if !r.Ping() {
		return nil, "", fmt.Errorf("Remote repository at %s does not exist, or is inaccessible", ustr)
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
	}, ustr, nil
}
