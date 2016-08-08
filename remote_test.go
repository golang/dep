package gps

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"testing"
)

func TestDeduceFromPath(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping remote deduction test in short mode")
	}

	cpath, err := ioutil.TempDir("", "smcache")
	if err != nil {
		t.Errorf("Failed to create temp dir: %s", err)
	}
	sm, err := NewSourceManager(naiveAnalyzer{}, cpath, false)

	if err != nil {
		t.Errorf("Unexpected error on SourceManager creation: %s", err)
		t.FailNow()
	}
	defer func() {
		err := removeAll(cpath)
		if err != nil {
			t.Errorf("removeAll failed: %s", err)
		}
	}()
	defer sm.Release()

	// helper func to generate testing *url.URLs, panicking on err
	mkurl := func(s string) (u *url.URL) {
		var err error
		u, err = url.Parse(s)
		if err != nil {
			panic(fmt.Sprint("string is not a valid URL:", s))
		}
		return
	}

	fixtures := []struct {
		in     string
		root   string
		rerr   error
		mb     maybeSource
		srcerr error
	}{
		{
			in:   "github.com/sdboyer/gps",
			root: "github.com/sdboyer/gps",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "github.com/sdboyer/gps/foo",
			root: "github.com/sdboyer/gps",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "github.com/sdboyer/gps.git/foo",
			root: "github.com/sdboyer/gps",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "git@github.com:sdboyer/gps",
			root: "github.com/sdboyer/gps",
			mb:   &maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
		},
		{
			in:   "https://github.com/sdboyer/gps",
			root: "github.com/sdboyer/gps",
			mb:   &maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
		},
		{
			in:   "https://github.com/sdboyer/gps/foo/bar",
			root: "github.com/sdboyer/gps",
			mb:   &maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
		},
		// some invalid github username patterns
		{
			in:   "github.com/-sdboyer/gps/foo",
			rerr: errors.New("github.com/-sdboyer/gps/foo is not a valid path for a source on github.com"),
		},
		{
			in:   "github.com/sdboyer-/gps/foo",
			rerr: errors.New("github.com/sdboyer-/gps/foo is not a valid path for a source on github.com"),
		},
		{
			in:   "github.com/sdbo.yer/gps/foo",
			rerr: errors.New("github.com/sdbo.yer/gps/foo is not a valid path for a source on github.com"),
		},
		{
			in:   "github.com/sdbo_yer/gps/foo",
			rerr: errors.New("github.com/sdbo_yer/gps/foo is not a valid path for a source on github.com"),
		},
		{
			in:   "gopkg.in/sdboyer/gps.v0",
			root: "gopkg.in/sdboyer/gps.v0",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "gopkg.in/sdboyer/gps.v0/foo",
			root: "gopkg.in/sdboyer/gps.v0",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "gopkg.in/sdboyer/gps.v1/foo/bar",
			root: "gopkg.in/sdboyer/gps.v1",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				&maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "gopkg.in/yaml.v1",
			root: "gopkg.in/yaml.v1",
			mb:   &maybeGitSource{url: mkurl("https://github.com/go-yaml/yaml")},
		},
		{
			in:   "gopkg.in/yaml.v1/foo/bar",
			root: "gopkg.in/yaml.v1",
			mb:   &maybeGitSource{url: mkurl("https://github.com/go-yaml/yaml")},
		},
		{
			// gopkg.in only allows specifying major version in import path
			root: "gopkg.in/yaml.v1.2",
			rerr: errors.New("gopkg.in/yaml.v1.2 is not a valid path; gopkg.in only allows major versions (\"v1\" instead of \"v1.2\")"),
		},
		// IBM hub devops services - fixtures borrowed from go get
		{
			in:   "hub.jazz.net/git/user1/pkgname",
			root: "hub.jazz.net/git/user1/pkgname",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("ssh://git@hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("git://hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("http://hub.jazz.net/git/user1/pkgname")},
			},
		},
		{
			in:   "hub.jazz.net/git/user1/pkgname/submodule/submodule/submodule",
			root: "hub.jazz.net/git/user1/pkgname",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("ssh://git@hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("git://hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("http://hub.jazz.net/git/user1/pkgname")},
			},
		},
		{
			in:   "hub.jazz.net",
			rerr: errors.New("unable to deduce repository and source type for: \"hub.jazz.net\""),
		},
		{
			in:   "hub2.jazz.net",
			rerr: errors.New("unable to deduce repository and source type for: \"hub2.jazz.net\""),
		},
		{
			in:   "hub.jazz.net/someotherprefix",
			rerr: errors.New("unable to deduce repository and source type for: \"hub.jazz.net/someotherprefix\""),
		},
		{
			in:   "hub.jazz.net/someotherprefix/user1/packagename",
			rerr: errors.New("unable to deduce repository and source type for: \"hub.jazz.net/someotherprefix/user1/packagename\""),
		},
		// Spaces are not valid in user names or package names
		{
			in:   "hub.jazz.net/git/User 1/pkgname",
			rerr: errors.New("hub.jazz.net/git/User 1/pkgname is not a valid path for a source on hub.jazz.net"),
		},
		{
			in:   "hub.jazz.net/git/user1/pkg name",
			rerr: errors.New("hub.jazz.net/git/user1/pkg name is not a valid path for a source on hub.jazz.net"),
		},
		// Dots are not valid in user names
		{
			in:   "hub.jazz.net/git/user.1/pkgname",
			rerr: errors.New("hub.jazz.net/git/user.1/pkgname is not a valid path for a source on hub.jazz.net"),
		},
		{
			in:   "hub.jazz.net/git/user/pkg.name",
			root: "hub.jazz.net/git/user/pkg.name",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("ssh://git@hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("git://hub.jazz.net/git/user1/pkgname")},
				&maybeGitSource{url: mkurl("http://hub.jazz.net/git/user1/pkgname")},
			},
		},
		// User names cannot have uppercase letters
		{
			in:   "hub.jazz.net/git/USER/pkgname",
			rerr: errors.New("hub.jazz.net/git/USER/pkgname is not a valid path for a source on hub.jazz.net"),
		},
		{
			in:   "bitbucket.org/sdboyer/reporoot",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "bitbucket.org/sdboyer/reporoot/foo/bar",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "https://bitbucket.org/sdboyer/reporoot/foo/bar",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
			},
		},
		// Less standard behaviors possible due to the hg/git ambiguity
		{
			in:   "bitbucket.org/sdboyer/reporoot.git",
			root: "bitbucket.org/sdboyer/reporoot.git",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				&maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "git@bitbucket.org:sdboyer/reporoot.git",
			root: "bitbucket.org/sdboyer/reporoot.git",
			mb:   &maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
		},
		{
			in:   "bitbucket.org/sdboyer/reporoot.hg",
			root: "bitbucket.org/sdboyer/reporoot.hg",
			mb: maybeSources{
				&maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				&maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "hg@bitbucket.org:sdboyer/reporoot",
			root: "bitbucket.org/sdboyer/reporoot",
			mb:   &maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
		},
		{
			in:     "git://bitbucket.org/sdboyer/reporoot.hg",
			root:   "bitbucket.org/sdboyer/reporoot.hg",
			srcerr: errors.New("git is not a valid scheme for accessing an hg repository"),
		},
		// tests for launchpad, mostly bazaar
		// TODO(sdboyer) need more tests to deal w/launchpad's oddities
		{
			in:   "launchpad.net/govcstestbzrrepo",
			root: "launchpad.net/govcstestbzrrepo",
			mb: maybeSources{
				&maybeBzrSource{url: mkurl("https://launchpad.net/govcstestbzrrepo")},
				&maybeBzrSource{url: mkurl("bzr://launchpad.net/govcstestbzrrepo")},
				&maybeBzrSource{url: mkurl("http://launchpad.net/govcstestbzrrepo")},
			},
		},
		{
			in:   "launchpad.net/govcstestbzrrepo/foo/bar",
			root: "launchpad.net/govcstestbzrrepo",
			mb: maybeSources{
				&maybeBzrSource{url: mkurl("https://launchpad.net/govcstestbzrrepo")},
				&maybeBzrSource{url: mkurl("bzr://launchpad.net/govcstestbzrrepo")},
				&maybeBzrSource{url: mkurl("http://launchpad.net/govcstestbzrrepo")},
			},
		},
		{
			in:   "launchpad.net/repo root",
			rerr: errors.New("launchpad.net/repo root is not a valid path for a source on launchpad.net"),
		},
		{
			in:   "git.launchpad.net/reporoot",
			root: "git.launchpad.net/reporoot",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://git.launchpad.net/reporoot")},
				&maybeGitSource{url: mkurl("ssh://git@git.launchpad.net/reporoot")},
				&maybeGitSource{url: mkurl("git://git.launchpad.net/reporoot")},
				&maybeGitSource{url: mkurl("http://git.launchpad.net/reporoot")},
			},
		},
		{
			in:   "git.launchpad.net/reporoot/foo/bar",
			root: "git.launchpad.net/reporoot",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://git.launchpad.net/reporoot")},
				&maybeGitSource{url: mkurl("ssh://git@git.launchpad.net/reporoot")},
				&maybeGitSource{url: mkurl("git://git.launchpad.net/reporoot")},
				&maybeGitSource{url: mkurl("http://git.launchpad.net/reporoot")},
			},
		},
		{
			in:   "git.launchpad.net/repo root",
			rerr: errors.New("git.launchpad.net/repo root is not a valid path for a source on launchpad.net"),
		},
		{
			in:   "git.apache.org/package-name.git",
			root: "git.apache.org/package-name.git",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://git.apache.org/package-name.git")},
				&maybeGitSource{url: mkurl("ssh://git@git.apache.org/package-name.git")},
				&maybeGitSource{url: mkurl("git://git.apache.org/package-name.git")},
				&maybeGitSource{url: mkurl("http://git.apache.org/package-name.git")},
			},
		},
		{
			in:   "git.apache.org/package-name.git/foo/bar",
			root: "git.apache.org/package-name.git",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://git.apache.org/package-name.git")},
				&maybeGitSource{url: mkurl("ssh://git@git.apache.org/package-name.git")},
				&maybeGitSource{url: mkurl("git://git.apache.org/package-name.git")},
				&maybeGitSource{url: mkurl("http://git.apache.org/package-name.git")},
			},
		},
		// Vanity imports
		{
			in:   "golang.org/x/exp",
			root: "golang.org/x/exp",
			mb:   &maybeGitSource{url: mkurl("https://go.googlesource.com/exp")},
		},
		{
			in:   "golang.org/x/exp/inotify",
			root: "golang.org/x/exp",
			mb:   &maybeGitSource{url: mkurl("https://go.googlesource.com/exp")},
		},
		{
			in:   "rsc.io/pdf",
			root: "rsc.io/pdf",
			mb:   &maybeGitSource{url: mkurl("https://github.com/rsc/pdf")},
		},
		// Regression - gh does allow two-letter usernames
		{
			in:   "github.com/kr/pretty",
			root: "github.com/kr/pretty",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://github.com/kr/pretty")},
				&maybeGitSource{url: mkurl("ssh://git@github.com/kr/pretty")},
				&maybeGitSource{url: mkurl("git://github.com/kr/pretty")},
				&maybeGitSource{url: mkurl("http://github.com/kr/pretty")},
			},
		},
		// VCS extension-based syntax
		{
			in:   "foobar/baz.git",
			root: "foobar/baz.git",
			mb: maybeSources{
				&maybeGitSource{url: mkurl("https://foobar/baz.git")},
				&maybeGitSource{url: mkurl("git://foobar/baz.git")},
				&maybeGitSource{url: mkurl("http://foobar/baz.git")},
			},
		},
		{
			in:   "foobar/baz.git/quark/quizzle.git",
			rerr: errors.New("not allowed: foobar/baz.git/quark/quizzle.git contains multiple vcs extension hints"),
		},
	}

	// TODO(sdboyer) this is all the old checking logic; convert it
	//for _, fix := range fixtures {
	//got, err := deduceRemoteRepo(fix.path)
	//want := fix.want

	//if want == nil {
	//if err == nil {
	//t.Errorf("deduceRemoteRepo(%q): Error expected but not received", fix.path)
	//}
	//continue
	//}

	//if err != nil {
	//t.Errorf("deduceRemoteRepo(%q): %v", fix.path, err)
	//continue
	//}

	//if got.Base != want.Base {
	//t.Errorf("deduceRemoteRepo(%q): Base was %s, wanted %s", fix.path, got.Base, want.Base)
	//}
	//if got.RelPkg != want.RelPkg {
	//t.Errorf("deduceRemoteRepo(%q): RelPkg was %s, wanted %s", fix.path, got.RelPkg, want.RelPkg)
	//}
	//if !reflect.DeepEqual(got.CloneURL, want.CloneURL) {
	//// misspelling things is cool when it makes columns line up
	//t.Errorf("deduceRemoteRepo(%q): CloneURL disagreement:\n(GOT) %s\n(WNT) %s", fix.path, ufmt(got.CloneURL), ufmt(want.CloneURL))
	//}
	//if !reflect.DeepEqual(got.VCS, want.VCS) {
	//t.Errorf("deduceRemoteRepo(%q): VCS was %s, wanted %s", fix.path, got.VCS, want.VCS)
	//}
	//if !reflect.DeepEqual(got.Schemes, want.Schemes) {
	//t.Errorf("deduceRemoteRepo(%q): Schemes was %s, wanted %s", fix.path, got.Schemes, want.Schemes)
	//}
	//}
	t.Error("TODO implement checking of new path deduction fixtures")
}

// borrow from stdlib
// more useful string for debugging than fmt's struct printer
func ufmt(u *url.URL) string {
	var user, pass interface{}
	if u.User != nil {
		user = u.User.Username()
		if p, ok := u.User.Password(); ok {
			pass = p
		}
	}
	return fmt.Sprintf("host=%q, path=%q, opaque=%q, scheme=%q, user=%#v, pass=%#v, rawpath=%q, rawq=%q, frag=%q",
		u.Host, u.Path, u.Opaque, u.Scheme, user, pass, u.RawPath, u.RawQuery, u.Fragment)
}
