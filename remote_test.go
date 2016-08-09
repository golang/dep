package gps

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
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
			in:   "github.com/sdboyer/gps.git/foo",
			root: "github.com/sdboyer/gps",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
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
				maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "gopkg.in/sdboyer/gps.v0/foo",
			root: "gopkg.in/sdboyer/gps.v0",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "gopkg.in/sdboyer/gps.v1/foo/bar",
			root: "gopkg.in/sdboyer/gps.v1",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("ssh://git@github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("git://github.com/sdboyer/gps")},
				maybeGitSource{url: mkurl("http://github.com/sdboyer/gps")},
			},
		},
		{
			in:   "gopkg.in/yaml.v1",
			root: "gopkg.in/yaml.v1",
			mb:   maybeGitSource{url: mkurl("https://github.com/go-yaml/yaml")},
		},
		{
			in:   "gopkg.in/yaml.v1/foo/bar",
			root: "gopkg.in/yaml.v1",
			mb:   maybeGitSource{url: mkurl("https://github.com/go-yaml/yaml")},
		},
		{
			// gopkg.in only allows specifying major version in import path
			in:   "gopkg.in/yaml.v1.2",
			rerr: errors.New("gopkg.in/yaml.v1.2 is not a valid path; gopkg.in only allows major versions (\"v1\" instead of \"v1.2\")"),
		},
	},
	"jazz": []pathDeductionFixture{
		// IBM hub devops services - fixtures borrowed from go get
		{
			in:   "hub.jazz.net/git/user1/pkgname",
			root: "hub.jazz.net/git/user1/pkgname",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("ssh://git@hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("git://hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("http://hub.jazz.net/git/user1/pkgname")},
			},
		},
		{
			in:   "hub.jazz.net/git/user1/pkgname/submodule/submodule/submodule",
			root: "hub.jazz.net/git/user1/pkgname",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("ssh://git@hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("git://hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("http://hub.jazz.net/git/user1/pkgname")},
			},
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
				maybeGitSource{url: mkurl("https://hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("ssh://git@hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("git://hub.jazz.net/git/user1/pkgname")},
				maybeGitSource{url: mkurl("http://hub.jazz.net/git/user1/pkgname")},
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
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
	},
	"bitbucket": []pathDeductionFixture{
		{
			in:   "bitbucket.org/sdboyer/reporoot/foo/bar",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "https://bitbucket.org/sdboyer/reporoot/foo/bar",
			root: "bitbucket.org/sdboyer/reporoot",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
			},
		},
		// Less standard behaviors possible due to the hg/git ambiguity
		{
			in:   "bitbucket.org/sdboyer/reporoot.git",
			root: "bitbucket.org/sdboyer/reporoot.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("git://bitbucket.org/sdboyer/reporoot")},
				maybeGitSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
			},
		},
		{
			in:   "git@bitbucket.org:sdboyer/reporoot.git",
			root: "bitbucket.org/sdboyer/reporoot.git",
			mb:   maybeGitSource{url: mkurl("ssh://git@bitbucket.org/sdboyer/reporoot")},
		},
		{
			in:   "bitbucket.org/sdboyer/reporoot.hg",
			root: "bitbucket.org/sdboyer/reporoot.hg",
			mb: maybeSources{
				maybeHgSource{url: mkurl("https://bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("ssh://hg@bitbucket.org/sdboyer/reporoot")},
				maybeHgSource{url: mkurl("http://bitbucket.org/sdboyer/reporoot")},
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
				maybeBzrSource{url: mkurl("bzr://launchpad.net/govcstestbzrrepo")},
				maybeBzrSource{url: mkurl("http://launchpad.net/govcstestbzrrepo")},
			},
		},
		{
			in:   "launchpad.net/govcstestbzrrepo/foo/bar",
			root: "launchpad.net/govcstestbzrrepo",
			mb: maybeSources{
				maybeBzrSource{url: mkurl("https://launchpad.net/govcstestbzrrepo")},
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
				maybeGitSource{url: mkurl("ssh://git@git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("git://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("http://git.launchpad.net/reporoot")},
			},
		},
		{
			in:   "git.launchpad.net/reporoot/foo/bar",
			root: "git.launchpad.net/reporoot",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://git.launchpad.net/reporoot")},
				maybeGitSource{url: mkurl("ssh://git@git.launchpad.net/reporoot")},
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
				maybeGitSource{url: mkurl("ssh://git@git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("git://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("http://git.apache.org/package-name.git")},
			},
		},
		{
			in:   "git.apache.org/package-name.git/foo/bar",
			root: "git.apache.org/package-name.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("ssh://git@git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("git://git.apache.org/package-name.git")},
				maybeGitSource{url: mkurl("http://git.apache.org/package-name.git")},
			},
		},
	},
	"vcsext": []pathDeductionFixture{
		// VCS extension-based syntax
		{
			in:   "foobar/baz.git",
			root: "foobar/baz.git",
			mb: maybeSources{
				maybeGitSource{url: mkurl("https://foobar/baz.git")},
				maybeGitSource{url: mkurl("git://foobar/baz.git")},
				maybeGitSource{url: mkurl("http://foobar/baz.git")},
			},
		},
		{
			in:   "foobar/baz.bzr",
			root: "foobar/baz.bzr",
			mb: maybeSources{
				maybeBzrSource{url: mkurl("https://foobar/baz.bzr")},
				maybeBzrSource{url: mkurl("bzr://foobar/baz.bzr")},
				maybeBzrSource{url: mkurl("http://foobar/baz.bzr")},
			},
		},
		{
			in:   "foobar/baz.hg",
			root: "foobar/baz.hg",
			mb: maybeSources{
				maybeHgSource{url: mkurl("https://foobar/baz.hg")},
				maybeHgSource{url: mkurl("http://foobar/baz.hg")},
			},
		},
		{
			in:   "foobar/baz.git/quark/quizzle.git",
			rerr: errors.New("not allowed: foobar/baz.git/quark/quizzle.git contains multiple vcs extension hints"),
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
		{
			in:   "rsc.io/pdf",
			root: "rsc.io/pdf",
			mb:   maybeGitSource{url: mkurl("https://github.com/rsc/pdf")},
		},
	},
}

func TestDeduceFromPath(t *testing.T) {
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
			if u == nil {
				spew.Dump(fix, uerr)
			}

			root, rerr := deducer.deduceRoot(in)
			if fix.rerr != nil {
				if fix.rerr != rerr {
					if rerr == nil {
						t.Errorf("(in: %s, %T) Expected error on deducing root, got none:\n\t(WNT) %s", in, deducer, fix.rerr)
					} else {
						t.Errorf("(in: %s, %T) Got unexpected error on deducing root:\n\t(GOT) %s\n\t(WNT) %s", in, deducer, rerr, fix.rerr)
					}
				}
			} else if rerr != nil {
				t.Errorf("(in: %s, %T) Got unexpected error on deducing root:\n\t(GOT) %s", in, deducer, rerr)
			} else if root != fix.root {
				t.Errorf("(in: %s, %T) Deducer did not return expected root:\n\t(GOT) %s\n\t(WNT) %s", in, deducer, root, fix.root)
			}

			mb, mberr := deducer.deduceSource(fix.in, u)
			if fix.srcerr != nil {
				if fix.srcerr != mberr {
					if mberr == nil {
						t.Errorf("(in: %s, %T) Expected error on deducing source, got none:\n\t(WNT) %s", in, deducer, fix.srcerr)
					} else {
						t.Errorf("(in: %s, %T) Got unexpected error on deducing source:\n\t(GOT) %s\n\t(WNT) %s", in, deducer, mberr, fix.srcerr)
					}
				}
			} else if mberr != nil && fix.rerr == nil { // don't complain the fix already expected an rerr
				t.Errorf("(in: %s, %T) Got unexpected error on deducing source:\n\t(GOT) %s", in, deducer, mberr)
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
