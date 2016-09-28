package gps

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/Masterminds/vcs"
)

// A maybeSource represents a set of information that, given some
// typically-expensive network effort, could be transformed into a proper source.
//
// Wrapping these up as their own type kills two birds with one stone:
//
// * Allows control over when deduction logic triggers network activity
// * Makes it easy to attempt multiple URLs for a given import path
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

	src.baseVCSSource.lvfunc = src.listVersions

	_, err = src.listVersions()
	if err != nil {
		return nil, "", err
	}

	return src, ustr, nil
}

type maybeGopkginSource struct {
	url   *url.URL
	major int64
}

func (m maybeGopkginSource) try(cachedir string, an ProjectAnalyzer) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewGitRepo(ustr, path)
	if err != nil {
		return nil, "", err
	}

	src := &gopkginSource{
		gitSource: gitSource{
			baseVCSSource: baseVCSSource{
				an: an,
				dc: newMetaCache(),
				crepo: &repo{
					r:     r,
					rpath: path,
				},
			},
		},
		major: m.major,
	}

	src.baseVCSSource.lvfunc = src.listVersions

	_, err = src.listVersions()
	if err != nil {
		return nil, "", err
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

	src := &bzrSource{
		baseVCSSource: baseVCSSource{
			an: an,
			dc: newMetaCache(),
			ex: existence{
				s: existsUpstream,
				f: existsUpstream,
			},
			crepo: &repo{
				r:     r,
				rpath: path,
			},
		},
	}
	src.baseVCSSource.lvfunc = src.listVersions

	return src, ustr, nil
}

type maybeHgSource struct {
	url *url.URL
}

func (m maybeHgSource) try(cachedir string, an ProjectAnalyzer) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewHgRepo(ustr, path)
	if err != nil {
		return nil, "", err
	}
	if !r.Ping() {
		return nil, "", fmt.Errorf("Remote repository at %s does not exist, or is inaccessible", ustr)
	}

	src := &hgSource{
		baseVCSSource: baseVCSSource{
			an: an,
			dc: newMetaCache(),
			ex: existence{
				s: existsUpstream,
				f: existsUpstream,
			},
			crepo: &repo{
				r:     r,
				rpath: path,
			},
		},
	}
	src.baseVCSSource.lvfunc = src.listVersions

	return src, ustr, nil
}
