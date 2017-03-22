package gps

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/Masterminds/vcs"
)

// A maybeSource represents a set of information that, given some
// typically-expensive network effort, could be transformed into a proper source.
//
// Wrapping these up as their own type achieves two goals:
//
// * Allows control over when deduction logic triggers network activity
// * Makes it easy to attempt multiple URLs for a given import path
type maybeSource interface {
	// TODO(sdboyer) remove ProjectAnalyzer from here after refactor to bring it in on
	// GetManifestAndLock() calls as a param
	try(ctx context.Context, cachedir string, c singleSourceCache) (source, string, error)
}

type maybeSources []maybeSource

func (mbs maybeSources) try(ctx context.Context, cachedir string, c singleSourceCache) (source, string, error) {
	var e sourceFailures
	for _, mb := range mbs {
		src, ident, err := mb.try(ctx, cachedir, c)
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
	fmt.Fprintf(&buf, "no valid source could be created:")
	for _, e := range sf {
		fmt.Fprintf(&buf, "\n\t%s", e.Error())
	}

	return buf.String()
}

type maybeGitSource struct {
	url *url.URL
}

func (m maybeGitSource) try(ctx context.Context, cachedir string, c singleSourceCache) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewGitRepo(ustr, path)
	if err != nil {
		return nil, ustr, unwrapVcsErr(err)
	}

	src := &gitSource{
		baseVCSSource: baseVCSSource{
			dc: c,
			crepo: &repo{
				r:     &gitRepo{r},
				rpath: path,
			},
		},
	}

	src.baseVCSSource.lvfunc = src.listVersions
	if !r.CheckLocal() {
		_, err = src.listVersions()
		if err != nil {
			return nil, ustr, unwrapVcsErr(err)
		}
	}

	return src, ustr, nil
}

type maybeGopkginSource struct {
	// the original gopkg.in import path. this is used to create the on-disk
	// location to avoid duplicate resource management - e.g., if instances of
	// a gopkg.in project are accessed via different schemes, or if the
	// underlying github repository is accessed directly.
	opath string
	// the actual upstream URL - always github
	url *url.URL
	// the major version to apply for filtering
	major uint64
}

func (m maybeGopkginSource) try(ctx context.Context, cachedir string, c singleSourceCache) (source, string, error) {
	// We don't actually need a fully consistent transform into the on-disk path
	// - just something that's unique to the particular gopkg.in domain context.
	// So, it's OK to just dumb-join the scheme with the path.
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(m.url.Scheme+"/"+m.opath))
	ustr := m.url.String()
	r, err := vcs.NewGitRepo(ustr, path)
	if err != nil {
		return nil, ustr, unwrapVcsErr(err)
	}

	src := &gopkginSource{
		gitSource: gitSource{
			baseVCSSource: baseVCSSource{
				dc: c,
				crepo: &repo{
					r:     &gitRepo{r},
					rpath: path,
				},
			},
		},
		major: m.major,
	}

	src.baseVCSSource.lvfunc = src.listVersions
	if !r.CheckLocal() {
		_, err = src.listVersions()
		if err != nil {
			return nil, ustr, unwrapVcsErr(err)
		}
	}

	return src, ustr, nil
}

type maybeBzrSource struct {
	url *url.URL
}

func (m maybeBzrSource) try(ctx context.Context, cachedir string, c singleSourceCache) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewBzrRepo(ustr, path)
	if err != nil {
		return nil, ustr, unwrapVcsErr(err)
	}
	if !r.Ping() {
		return nil, ustr, fmt.Errorf("remote repository at %s does not exist, or is inaccessible", ustr)
	}

	src := &bzrSource{
		baseVCSSource: baseVCSSource{
			dc: c,
			ex: existence{
				s: existsUpstream,
				f: existsUpstream,
			},
			crepo: &repo{
				r:     &bzrRepo{r},
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

func (m maybeHgSource) try(ctx context.Context, cachedir string, c singleSourceCache) (source, string, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))
	r, err := vcs.NewHgRepo(ustr, path)
	if err != nil {
		return nil, ustr, unwrapVcsErr(err)
	}
	if !r.Ping() {
		return nil, ustr, fmt.Errorf("remote repository at %s does not exist, or is inaccessible", ustr)
	}

	src := &hgSource{
		baseVCSSource: baseVCSSource{
			dc: c,
			ex: existence{
				s: existsUpstream,
				f: existsUpstream,
			},
			crepo: &repo{
				r:     &hgRepo{r},
				rpath: path,
			},
		},
	}
	src.baseVCSSource.lvfunc = src.listVersions

	return src, ustr, nil
}
