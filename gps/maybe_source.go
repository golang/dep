package gps

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

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
	try(ctx context.Context, cachedir string, c singleSourceCache, superv *supervisor) (source, sourceState, error)
	getURL() string
}

type maybeSources []maybeSource

func (mbs maybeSources) try(ctx context.Context, cachedir string, c singleSourceCache, superv *supervisor) (source, sourceState, error) {
	var e sourceFailures
	for _, mb := range mbs {
		src, state, err := mb.try(ctx, cachedir, c, superv)
		if err == nil {
			return src, state, nil
		}
		e = append(e, sourceSetupFailure{
			ident: mb.getURL(),
			err:   err,
		})
	}
	return nil, 0, e
}

// This really isn't generally intended to be used - the interface is for
// maybeSources to be able to interrogate its members, not other things to
// interrogate a maybeSources.
func (mbs maybeSources) getURL() string {
	strslice := make([]string, 0, len(mbs))
	for _, mb := range mbs {
		strslice = append(strslice, mb.getURL())
	}
	return strings.Join(strslice, "\n")
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

func (m maybeGitSource) try(ctx context.Context, cachedir string, c singleSourceCache, superv *supervisor) (source, sourceState, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))

	r, err := vcs.NewGitRepo(ustr, path)
	if err != nil {
		return nil, 0, unwrapVcsErr(err)
	}

	src := &gitSource{
		baseVCSSource: baseVCSSource{
			repo: &gitRepo{r},
		},
	}

	// Pinging invokes the same action as calling listVersions, so just do that.
	var vl []PairedVersion
	err = superv.do(ctx, "git:lv:maybe", ctListVersions, func(ctx context.Context) (err error) {
		if vl, err = src.listVersions(ctx); err != nil {
			return fmt.Errorf("remote repository at %s does not exist, or is inaccessible", ustr)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	c.storeVersionMap(vl, true)
	state := sourceIsSetUp | sourceExistsUpstream | sourceHasLatestVersionList

	if r.CheckLocal() {
		state |= sourceExistsLocally
	}

	return src, state, nil
}

func (m maybeGitSource) getURL() string {
	return m.url.String()
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

func (m maybeGopkginSource) try(ctx context.Context, cachedir string, c singleSourceCache, superv *supervisor) (source, sourceState, error) {
	// We don't actually need a fully consistent transform into the on-disk path
	// - just something that's unique to the particular gopkg.in domain context.
	// So, it's OK to just dumb-join the scheme with the path.
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(m.url.Scheme+"/"+m.opath))
	ustr := m.url.String()

	r, err := vcs.NewGitRepo(ustr, path)
	if err != nil {
		return nil, 0, unwrapVcsErr(err)
	}

	src := &gopkginSource{
		gitSource: gitSource{
			baseVCSSource: baseVCSSource{
				repo: &gitRepo{r},
			},
		},
		major: m.major,
	}

	var vl []PairedVersion
	err = superv.do(ctx, "git:lv:maybe", ctListVersions, func(ctx context.Context) (err error) {
		if vl, err = src.listVersions(ctx); err != nil {
			return fmt.Errorf("remote repository at %s does not exist, or is inaccessible", ustr)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	c.storeVersionMap(vl, true)
	state := sourceIsSetUp | sourceExistsUpstream | sourceHasLatestVersionList

	if r.CheckLocal() {
		state |= sourceExistsLocally
	}

	return src, state, nil
}

func (m maybeGopkginSource) getURL() string {
	return m.opath
}

type maybeBzrSource struct {
	url *url.URL
}

func (m maybeBzrSource) try(ctx context.Context, cachedir string, c singleSourceCache, superv *supervisor) (source, sourceState, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))

	r, err := vcs.NewBzrRepo(ustr, path)
	if err != nil {
		return nil, 0, unwrapVcsErr(err)
	}

	err = superv.do(ctx, "bzr:ping", ctSourcePing, func(ctx context.Context) error {
		if !r.Ping() {
			return fmt.Errorf("remote repository at %s does not exist, or is inaccessible", ustr)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	state := sourceIsSetUp | sourceExistsUpstream
	if r.CheckLocal() {
		state |= sourceExistsLocally
	}

	src := &bzrSource{
		baseVCSSource: baseVCSSource{
			repo: &bzrRepo{r},
		},
	}

	return src, state, nil
}

func (m maybeBzrSource) getURL() string {
	return m.url.String()
}

type maybeHgSource struct {
	url *url.URL
}

func (m maybeHgSource) try(ctx context.Context, cachedir string, c singleSourceCache, superv *supervisor) (source, sourceState, error) {
	ustr := m.url.String()
	path := filepath.Join(cachedir, "sources", sanitizer.Replace(ustr))

	r, err := vcs.NewHgRepo(ustr, path)
	if err != nil {
		return nil, 0, unwrapVcsErr(err)
	}

	err = superv.do(ctx, "hg:ping", ctSourcePing, func(ctx context.Context) error {
		if !r.Ping() {
			return fmt.Errorf("remote repository at %s does not exist, or is inaccessible", ustr)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	state := sourceIsSetUp | sourceExistsUpstream
	if r.CheckLocal() {
		state |= sourceExistsLocally
	}

	src := &hgSource{
		baseVCSSource: baseVCSSource{
			repo: &hgRepo{r},
		},
	}

	return src, state, nil
}

func (m maybeHgSource) getURL() string {
	return m.url.String()
}
