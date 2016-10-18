package gps

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sync"
	"testing"
)

type pathDeductionFixture struct {
	in     string
	root   string
	rerr   error
	mb     maybeSource
	srcerr error
}

// helper func to generate testing *url.URLs, panicking on err
func mkurl(s string) (u *url.URL) {
	var err error
	u, err = url.Parse(s)
	if err != nil {
		panic(fmt.Sprint("string is not a valid URL:", s))
	}
	return
}

var pathDeductionFixtures = map[string][]pathDeductionFixture{
	"github": []pathDeductionFixture{
		{
			in:   "github.com/sdboyer/gps",
			root: "github.com/sdboyer/gps",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "github.com/sdboyer/gps/foo",
			root: "github.com/sdboyer/gps",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			// TODO(sdboyer) is this a problem for enforcing uniqueness? do we
			// need to collapse these extensions?
			in:   "github.com/sdboyer/gps.git/foo",
			root: "github.com/sdboyer/gps.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://github.com/sdboyer/gps.git")},
				maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps.git")},
				maybeGitSource{url: mkurl("git://github.com/sdboyer/gps.git")},
				maybeGitSource{url: mkurl("http://github.com/sdboyer/gps.git")},
			},
		},
		{
			in:   "git@github.com:sdboyer/gps",
			root: "github.com/sdboyer/gps",
			mb:   maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
		},
		{
			in:   "https://github.com/sdboyer/gps",
			root: "github.com/sdboyer/gps",
			mb:   maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
		},
		{
			in:   "https://github.com/sdboyer/gps/foo/bar",
			root: "github.com/sdboyer/gps",
			mb:   maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
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
		// Regression - gh does allow two-letter usernames
		{
			in:   "github.com/kr/pretty",
			root: "github.com/kr/pretty",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://github.com/kr/pretty")},
				maybeGitSource{url: mkurl("ssh://git@github.com/kr/pretty")},
				maybeGitSource{url: mkurl("git://github.com/kr/pretty")},
				maybeGitSource{url: mkurl("http://github.com/kr/pretty")},
			},
		},
	},
	"gopkg.in": []pathDeductionFixture{
		{
			in:   "gopkg.in/sdboyer/gps.v0",
			root: "gopkg.in/sdboyer/gps.v0",
			mb: maybeSources{
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("https://github.com/sdboyer/gps"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("ssh://git@github.com/sdboyer/gps"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("git://github.com/sdboyer/gps"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("http://github.com/sdboyer/gps"), major: 0},
			},
		},
		{
			in:   "gopkg.in/sdboyer/gps.v0/foo",
			root: "gopkg.in/sdboyer/gps.v0",
			mb: maybeSources{
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("https://github.com/sdboyer/gps"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("ssh://git@github.com/sdboyer/gps"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("git://github.com/sdboyer/gps"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v0", url: mkurl("http://github.com/sdboyer/gps"), major: 0},
			},
		},
		{
			in:   "gopkg.in/sdboyer/gps.v1/foo/bar",
			root: "gopkg.in/sdboyer/gps.v1",
			mb: maybeSources{
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v1", url: mkurl("https://github.com/sdboyer/gps"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v1", url: mkurl("ssh://git@github.com/sdboyer/gps"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v1", url: mkurl("git://github.com/sdboyer/gps"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/sdboyer/gps.v1", url: mkurl("http://github.com/sdboyer/gps"), major: 1},
			},
		},
		{
			in:   "gopkg.in/yaml.v1",
			root: "gopkg.in/yaml.v1",
			mb: maybeSources{
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("https://github.com/go-yaml/yaml"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("ssh://git@github.com/go-yaml/yaml"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("git://github.com/go-yaml/yaml"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("http://github.com/go-yaml/yaml"), major: 1},
			},
		},
		{
			in:   "gopkg.in/yaml.v1/foo/bar",
			root: "gopkg.in/yaml.v1",
			mb: maybeSources{
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("https://github.com/go-yaml/yaml"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("ssh://git@github.com/go-yaml/yaml"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("git://github.com/go-yaml/yaml"), major: 1},
				maybeGopkginSource{opath: "gopkg.in/yaml.v1", url: mkurl("http://github.com/go-yaml/yaml"), major: 1},
			},
		},
		{
			in:   "gopkg.in/inf.v0",
			root: "gopkg.in/inf.v0",
			mb: maybeSources{
				maybeGopkginSource{opath: "gopkg.in/inf.v0", url: mkurl("https://github.com/go-inf/inf"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/inf.v0", url: mkurl("ssh://git@github.com/go-inf/inf"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/inf.v0", url: mkurl("git://github.com/go-inf/inf"), major: 0},
				maybeGopkginSource{opath: "gopkg.in/inf.v0", url: mkurl("http://github.com/go-inf/inf"), major: 0},
			},
		},
		{
			// gopkg.in only allows specifying major version in import path
			in:   "gopkg.in/yaml.v1.2",
			rerr: errors.New("gopkg.in/yaml.v1.2 is not a valid import path; gopkg.in only allows major versions (\"v1\" instead of \"v1.2\")"),
		},
	},
	"jazz": []pathDeductionFixture{
		// IBM hub devops services - fixtures borrowed from go get
		{
			in:   "hub.jazz.net/git/user1/pkgname",
			root: "hub.jazz.net/git/user1/pkgname",
			mb:   maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
		},
		{
			in:   "hub.jazz.net/git/user1/pkgname/submodule/submodule/submodule",
			root: "hub.jazz.net/git/user1/pkgname",
			mb:   maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
		},
		{
			in:   "hub.jazz.net/someotherprefix",
			rerr: errors.New("hub.jazz.net/someotherprefix is not a valid path for a source on hub.jazz.net"),
		},
		{
			in:   "hub.jazz.net/someotherprefix/user1/packagename",
			rerr: errors.New("hub.jazz.net/someotherprefix/user1/packagename is not a valid path for a source on hub.jazz.net"),
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
			in:   "hub.jazz.net/git/user1/pkg.name",
			root: "hub.jazz.net/git/user1/pkg.name",
			mb:   maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkg.name")},
		},
		// User names cannot have uppercase letters
		{
			in:   "hub.jazz.net/git/USER/pkgname",
			rerr: errors.New("hub.jazz.net/git/USER/pkgname is not a valid path for a source on hub.jazz.net"),
		},
	},
	"bitbucket": []pathDeductionFixture{
		{
			in:   "bitbucket.org/sdboyer/reporoot",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "bitbucket.org/sdboyer/reporoot/foo/bar",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "https://bitbucket.org/sdboyer/reporoot/foo/bar",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
			},
		},
		// Less standard behaviors possible due to the hg/git ambiguity
		{
			in:   "bitbucket.org/sdboyer/reporoot.git",
			root: "bitbucket.org/sdboyer/reporoot.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot.git")},
				maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot.git")},
				maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot.git")},
				maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot.git")},
			},
		},
		{
			in:   "git@bitbucket.org:sdboyer/reporoot.git",
			root: "bitbucket.org/sdboyer/reporoot.git",
			mb:   maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot.git")},
		},
		{
			in:   "bitbucket.org/sdboyer/reporoot.hg",
			root: "bitbucket.org/sdboyer/reporoot.hg",
			mb: maybeSources{
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot.hg")},
				maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot.hg")},
				maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot.hg")},
			},
		},
		{
			in:   "hg@bitbucket.org:sdboyer/reporoot",
			root: "bitbucket.org/sdboyer/reporoot",
			mb:   maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
		},
		{
			in:     "git://bitbucket.org/sdboyer/reporoot.hg",
			root:   "bitbucket.org/sdboyer/reporoot.hg",
			srcerr: errors.New("git is not a valid scheme for accessing an hg repository"),
		},
	},
	"launchpad": []pathDeductionFixture{
		// tests for launchpad, mostly bazaar
		// TODO(sdboyer) need more tests to deal w/launchpad's oddities
		{
			in:   "launchpad.net/govcstestbzrrepo",
			root: "launchpad.net/govcstestbzrrepo",
			mb: maybeSources{
				maybeBzrSource{url: mkurl("https://launchpad.net/govcstestbzrrepo")},
				maybeBzrSource{url: mkurl("bzr+ssh://launchpad.net/govcstestbzrrepo")},
				maybeBzrSource{url: mkurl("bzr://launchpad.net/govcstestbzrrepo")},
				maybeBzrSource{url: mkurl("http://launchpad.net/govcstestbzrrepo")},
			},
		},
		{
			in:   "launchpad.net/govcstestbzrrepo/foo/bar",
			root: "launchpad.net/govcstestbzrrepo",
			mb: maybeSources{
				maybeBzrSource{url: mkurl("https://launchpad.net/govcstestbzrrepo")},
				maybeBzrSource{url: mkurl("bzr+ssh://launchpad.net/govcstestbzrrepo")},
				maybeBzrSource{url: mkurl("bzr://launchpad.net/govcstestbzrrepo")},
				maybeBzrSource{url: mkurl("http://launchpad.net/govcstestbzrrepo")},
			},
		},
		{
			in:   "launchpad.net/repo root",
			rerr: errors.New("launchpad.net/repo root is not a valid path for a source on launchpad.net"),
		},
	},
	"git.launchpad": []pathDeductionFixture{
		{
			in:   "git.launchpad.net/reporoot",
			root: "git.launchpad.net/reporoot",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("ssh://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("git://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("http://git.launchpad.net/reporoot")},
			},
		},
		{
			in:   "git.launchpad.net/reporoot/foo/bar",
			root: "git.launchpad.net/reporoot",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("ssh://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("git://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("http://git.launchpad.net/reporoot")},
			},
		},
		{
			in:   "git.launchpad.net/repo root",
			rerr: errors.New("git.launchpad.net/repo root is not a valid path for a source on launchpad.net"),
		},
	},
	"apache": []pathDeductionFixture{
		{
			in:   "git.apache.org/package-name.git",
			root: "git.apache.org/package-name.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("ssh://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("git://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("http://git.apache.org/package-name.git")},
			},
		},
		{
			in:   "git.apache.org/package-name.git/foo/bar",
			root: "git.apache.org/package-name.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("ssh://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("git://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("http://git.apache.org/package-name.git")},
			},
		},
	},
	"vcsext": []pathDeductionFixture{
		// VCS extension-based syntax
		{
			in:   "foobar.com/baz.git",
			root: "foobar.com/baz.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("ssh://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("git://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("http://foobar.com/baz.git")},
			},
		},
		{
			in:   "foobar.com/baz.git/extra/path",
			root: "foobar.com/baz.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("ssh://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("git://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("http://foobar.com/baz.git")},
			},
		},
		{
			in:   "foobar.com/baz.bzr",
			root: "foobar.com/baz.bzr",
			mb: maybeSources{
				maybeBzrSource{url: mkurl("https://foobar.com/baz.bzr")},
				maybeBzrSource{url: mkurl("bzr+ssh://foobar.com/baz.bzr")},
				maybeBzrSource{url: mkurl("bzr://foobar.com/baz.bzr")},
				maybeBzrSource{url: mkurl("http://foobar.com/baz.bzr")},
			},
		},
		{
			in:   "foo-bar.com/baz.hg",
			root: "foo-bar.com/baz.hg",
			mb: maybeSources{
				maybeHgSource{url: mkurl("https://foo-bar.com/baz.hg")},
				maybeHgSource{url: mkurl("ssh://foo-bar.com/baz.hg")},
				maybeHgSource{url: mkurl("http://foo-bar.com/baz.hg")},
			},
		},
		{
			in:   "git@foobar.com:baz.git",
			root: "foobar.com/baz.git",
			mb:   maybeGitSource{url: mkurl("ssh://git@foobar.com/baz.git")},
		},
		{
			in:   "bzr+ssh://foobar.com/baz.bzr",
			root: "foobar.com/baz.bzr",
			mb:   maybeBzrSource{url: mkurl("bzr+ssh://foobar.com/baz.bzr")},
		},
		{
			in:   "ssh://foobar.com/baz.bzr",
			root: "foobar.com/baz.bzr",
			mb:   maybeBzrSource{url: mkurl("ssh://foobar.com/baz.bzr")},
		},
		{
			in:   "https://foobar.com/baz.hg",
			root: "foobar.com/baz.hg",
			mb:   maybeHgSource{url: mkurl("https://foobar.com/baz.hg")},
		},
		{
			in:     "git://foobar.com/baz.hg",
			root:   "foobar.com/baz.hg",
			srcerr: errors.New("git is not a valid scheme for accessing hg repositories (path foobar.com/baz.hg)"),
		},
		// who knows why anyone would do this, but having a second vcs ext
		// shouldn't throw us off - only the first one counts
		{
			in:   "foobar.com/baz.git/quark/quizzle.bzr/quorum",
			root: "foobar.com/baz.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("ssh://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("git://foobar.com/baz.git")},
				maybeGitSource{url: mkurl("http://foobar.com/baz.git")},
			},
		},
	},
	"vanity": []pathDeductionFixture{
		// Vanity imports
		{
			in:   "golang.org/x/exp",
			root: "golang.org/x/exp",
			mb:   maybeGitSource{url: mkurl("https://go.googlesource.com/exp")},
		},
		{
			in:   "golang.org/x/exp/inotify",
			root: "golang.org/x/exp",
			mb:   maybeGitSource{url: mkurl("https://go.googlesource.com/exp")},
		},
		// rsc.io appears to have broken
		//{
		//in:   "rsc.io/pdf",
		//root: "rsc.io/pdf",
		//mb:   maybeGitSource{url: mkurl("https://github.com/rsc/pdf")},
		//},
	},
}

func TestDeduceFromPath(t *testing.T) {
	for typ, fixtures := range pathDeductionFixtures {
		var deducer pathDeducer
		switch typ {
		case "github":
			deducer = githubDeducer{regexp: ghRegex}
		case "gopkg.in":
			deducer = gopkginDeducer{regexp: gpinNewRegex}
		case "jazz":
			deducer = jazzDeducer{regexp: jazzRegex}
		case "bitbucket":
			deducer = bitbucketDeducer{regexp: bbRegex}
		case "launchpad":
			deducer = launchpadDeducer{regexp: lpRegex}
		case "git.launchpad":
			deducer = launchpadGitDeducer{regexp: glpRegex}
		case "apache":
			deducer = apacheDeducer{regexp: apacheRegex}
		case "vcsext":
			deducer = vcsExtensionDeducer{regexp: vcsExtensionRegex}
		default:
			// Should just be the vanity imports, which we do elsewhere
			continue
		}

		var printmb func(mb maybeSource) string
		printmb = func(mb maybeSource) string {
			switch tmb := mb.(type) {
			case maybeSources:
				var buf bytes.Buffer
				fmt.Fprintf(&buf, "%v maybeSources:", len(tmb))
				for _, elem := range tmb {
					fmt.Fprintf(&buf, "\n\t\t%s", printmb(elem))
				}
				return buf.String()
			case maybeGitSource:
				return fmt.Sprintf("%T: %s", tmb, ufmt(tmb.url))
			case maybeBzrSource:
				return fmt.Sprintf("%T: %s", tmb, ufmt(tmb.url))
			case maybeHgSource:
				return fmt.Sprintf("%T: %s", tmb, ufmt(tmb.url))
			case maybeGopkginSource:
				return fmt.Sprintf("%T: %s (v%v) %s ", tmb, tmb.opath, tmb.major, ufmt(tmb.url))
			default:
				t.Errorf("Unknown maybeSource type: %T", mb)
				t.FailNow()
			}
			return ""
		}

		for _, fix := range fixtures {
			u, in, uerr := normalizeURI(fix.in)
			if uerr != nil {
				if fix.rerr == nil {
					t.Errorf("(in: %s) bad input URI %s", fix.in, uerr)
				}
				continue
			}

			root, rerr := deducer.deduceRoot(in)
			if fix.rerr != nil {
				if rerr == nil {
					t.Errorf("(in: %s, %T) Expected error on deducing root, got none:\n\t(WNT) %s", in, deducer, fix.rerr)
				} else if fix.rerr.Error() != rerr.Error() {
					t.Errorf("(in: %s, %T) Got unexpected error on deducing root:\n\t(GOT) %s\n\t(WNT) %s", in, deducer, rerr, fix.rerr)
				}
			} else if rerr != nil {
				t.Errorf("(in: %s, %T) Got unexpected error on deducing root:\n\t(GOT) %s", in, deducer, rerr)
			} else if root != fix.root {
				t.Errorf("(in: %s, %T) Deducer did not return expected root:\n\t(GOT) %s\n\t(WNT) %s", in, deducer, root, fix.root)
			}

			mb, mberr := deducer.deduceSource(in, u)
			if fix.srcerr != nil {
				if mberr == nil {
					t.Errorf("(in: %s, %T) Expected error on deducing source, got none:\n\t(WNT) %s", in, deducer, fix.srcerr)
				} else if fix.srcerr.Error() != mberr.Error() {
					t.Errorf("(in: %s, %T) Got unexpected error on deducing source:\n\t(GOT) %s\n\t(WNT) %s", in, deducer, mberr, fix.srcerr)
				}
			} else if mberr != nil {
				// don't complain the fix already expected an rerr
				if fix.rerr == nil {
					t.Errorf("(in: %s, %T) Got unexpected error on deducing source:\n\t(GOT) %s", in, deducer, mberr)
				}
			} else if !reflect.DeepEqual(mb, fix.mb) {
				if mb == nil {
					t.Errorf("(in: %s, %T) Deducer returned source maybes, but none expected:\n\t(GOT) (none)\n\t(WNT) %s", in, deducer, printmb(fix.mb))
				} else if fix.mb == nil {
					t.Errorf("(in: %s, %T) Deducer returned source maybes, but none expected:\n\t(GOT) %s\n\t(WNT) (none)", in, deducer, printmb(mb))
				} else {
					t.Errorf("(in: %s, %T) Deducer did not return expected source:\n\t(GOT) %s\n\t(WNT) %s", in, deducer, printmb(mb), printmb(fix.mb))
				}
			}
		}
	}
}

func TestVanityDeduction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow test in short mode")
	}

	sm, clean := mkNaiveSM(t)
	defer clean()

	vanities := pathDeductionFixtures["vanity"]
	wg := &sync.WaitGroup{}
	wg.Add(len(vanities))

	for _, fix := range vanities {
		go func(fix pathDeductionFixture) {
			defer wg.Done()
			pr, err := sm.DeduceProjectRoot(fix.in)
			if err != nil {
				t.Errorf("(in: %s) Unexpected err on deducing project root: %s", fix.in, err)
				return
			} else if string(pr) != fix.root {
				t.Errorf("(in: %s) Deducer did not return expected root:\n\t(GOT) %s\n\t(WNT) %s", fix.in, pr, fix.root)
			}

			ft, err := sm.deducePathAndProcess(fix.in)
			if err != nil {
				t.Errorf("(in: %s) Unexpected err on deducing source: %s", fix.in, err)
				return
			}

			_, ident, err := ft.srcf()
			if err != nil {
				t.Errorf("(in: %s) Unexpected err on executing source future: %s", fix.in, err)
				return
			}

			ustr := fix.mb.(maybeGitSource).url.String()
			if ident != ustr {
				t.Errorf("(in: %s) Deduced repo ident does not match fixture:\n\t(GOT) %s\n\t(WNT) %s", fix.in, ident, ustr)
			}
		}(fix)
	}

	wg.Wait()
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
