package gps

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
)

func TestDeduceRemotes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping remote deduction test in short mode")
	}

	fixtures := []struct {
		path string
		want *remoteRepo
	}{
		{
			"github.com/sdboyer/gps",
			&remoteRepo{
				repoRoot: "github.com/sdboyer/gps",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/gps",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"github.com/sdboyer/gps/foo",
			&remoteRepo{
				repoRoot: "github.com/sdboyer/gps",
				relPkg:   "foo",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/gps",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"git@github.com:sdboyer/gps",
			&remoteRepo{
				repoRoot: "github.com/sdboyer/gps",
				relPkg:   "",
				CloneURL: &url.URL{
					Scheme: "ssh",
					User:   url.User("git"),
					Host:   "github.com",
					Path:   "sdboyer/gps",
				},
				Schemes: []string{"ssh"},
				VCS:     []string{"git"},
			},
		},
		{
			"https://github.com/sdboyer/gps/foo",
			&remoteRepo{
				repoRoot: "github.com/sdboyer/gps",
				relPkg:   "foo",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
					Path:   "sdboyer/gps",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git"},
			},
		},
		{
			"https://github.com/sdboyer/gps/foo/bar",
			&remoteRepo{
				repoRoot: "github.com/sdboyer/gps",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
					Path:   "sdboyer/gps",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git"},
			},
		},
		// some invalid github username patterns
		{
			"github.com/-sdboyer/gps/foo",
			nil,
		},
		{
			"github.com/sdboyer-/gps/foo",
			nil,
		},
		{
			"github.com/sdbo.yer/gps/foo",
			nil,
		},
		{
			"github.com/sdbo_yer/gps/foo",
			nil,
		},
		{
			"gopkg.in/sdboyer/gps.v0",
			&remoteRepo{
				repoRoot: "gopkg.in/sdboyer/gps.v0",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/gps",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"gopkg.in/sdboyer/gps.v0/foo",
			&remoteRepo{
				repoRoot: "gopkg.in/sdboyer/gps.v0",
				relPkg:   "foo",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/gps",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"gopkg.in/sdboyer/gps.v0/foo/bar",
			&remoteRepo{
				repoRoot: "gopkg.in/sdboyer/gps.v0",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/gps",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"gopkg.in/yaml.v1",
			&remoteRepo{
				repoRoot: "gopkg.in/yaml.v1",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "go-pkg/yaml",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"gopkg.in/yaml.v1/foo/bar",
			&remoteRepo{
				repoRoot: "gopkg.in/yaml.v1",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "go-pkg/yaml",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			// gopkg.in only allows specifying major version in import path
			"gopkg.in/yaml.v1.2",
			nil,
		},
		// IBM hub devops services - fixtures borrowed from go get
		{
			"hub.jazz.net/git/user1/pkgname",
			&remoteRepo{
				repoRoot: "hub.jazz.net/git/user1/pkgname",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "hub.jazz.net",
					Path: "git/user1/pkgname",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"hub.jazz.net/git/user1/pkgname/submodule/submodule/submodule",
			&remoteRepo{
				repoRoot: "hub.jazz.net/git/user1/pkgname",
				relPkg:   "submodule/submodule/submodule",
				CloneURL: &url.URL{
					Host: "hub.jazz.net",
					Path: "git/user1/pkgname",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"hub.jazz.net",
			nil,
		},
		{
			"hub2.jazz.net",
			nil,
		},
		{
			"hub.jazz.net/someotherprefix",
			nil,
		},
		{
			"hub.jazz.net/someotherprefix/user1/pkgname",
			nil,
		},
		// Spaces are not valid in user names or package names
		{
			"hub.jazz.net/git/User 1/pkgname",
			nil,
		},
		{
			"hub.jazz.net/git/user1/pkg name",
			nil,
		},
		// Dots are not valid in user names
		{
			"hub.jazz.net/git/user.1/pkgname",
			nil,
		},
		{
			"hub.jazz.net/git/user/pkg.name",
			&remoteRepo{
				repoRoot: "hub.jazz.net/git/user/pkg.name",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "hub.jazz.net",
					Path: "git/user/pkg.name",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		// User names cannot have uppercase letters
		{
			"hub.jazz.net/git/USER/pkgname",
			nil,
		},
		{
			"bitbucket.org/sdboyer/reporoot",
			&remoteRepo{
				repoRoot: "bitbucket.org/sdboyer/reporoot",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "bitbucket.org",
					Path: "sdboyer/reporoot",
				},
				Schemes: hgSchemes,
				VCS:     []string{"git", "hg"},
			},
		},
		{
			"bitbucket.org/sdboyer/reporoot/foo/bar",
			&remoteRepo{
				repoRoot: "bitbucket.org/sdboyer/reporoot",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Host: "bitbucket.org",
					Path: "sdboyer/reporoot",
				},
				Schemes: hgSchemes,
				VCS:     []string{"git", "hg"},
			},
		},
		{
			"https://bitbucket.org/sdboyer/reporoot/foo/bar",
			&remoteRepo{
				repoRoot: "bitbucket.org/sdboyer/reporoot",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "bitbucket.org",
					Path:   "sdboyer/reporoot",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git", "hg"},
			},
		},
		{
			"launchpad.net/govcstestbzrrepo",
			&remoteRepo{
				repoRoot: "launchpad.net/govcstestbzrrepo",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "launchpad.net",
					Path: "govcstestbzrrepo",
				},
				Schemes: bzrSchemes,
				VCS:     []string{"bzr"},
			},
		},
		{
			"launchpad.net/govcstestbzrrepo/foo/bar",
			&remoteRepo{
				repoRoot: "launchpad.net/govcstestbzrrepo",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Host: "launchpad.net",
					Path: "govcstestbzrrepo",
				},
				Schemes: bzrSchemes,
				VCS:     []string{"bzr"},
			},
		},
		{
			"launchpad.net/repo root",
			nil,
		},
		{
			"git.launchpad.net/reporoot",
			&remoteRepo{
				repoRoot: "git.launchpad.net/reporoot",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "git.launchpad.net",
					Path: "reporoot",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"git.launchpad.net/reporoot/foo/bar",
			&remoteRepo{
				repoRoot: "git.launchpad.net/reporoot",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Host: "git.launchpad.net",
					Path: "reporoot",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"git.launchpad.net/reporoot",
			&remoteRepo{
				repoRoot: "git.launchpad.net/reporoot",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "git.launchpad.net",
					Path: "reporoot",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"git.launchpad.net/repo root",
			nil,
		},
		{
			"git.apache.org/package-name.git",
			&remoteRepo{
				repoRoot: "git.apache.org/package-name.git",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "git.apache.org",
					Path: "package-name.git",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		{
			"git.apache.org/package-name.git/foo/bar",
			&remoteRepo{
				repoRoot: "git.apache.org/package-name.git",
				relPkg:   "foo/bar",
				CloneURL: &url.URL{
					Host: "git.apache.org",
					Path: "package-name.git",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
		// Vanity imports
		{
			"golang.org/x/exp",
			&remoteRepo{
				repoRoot: "golang.org/x/exp",
				relPkg:   "",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "go.googlesource.com",
					Path:   "/exp",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git"},
			},
		},
		{
			"golang.org/x/exp/inotify",
			&remoteRepo{
				repoRoot: "golang.org/x/exp",
				relPkg:   "inotify",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "go.googlesource.com",
					Path:   "/exp",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git"},
			},
		},
		{
			"rsc.io/pdf",
			&remoteRepo{
				repoRoot: "rsc.io/pdf",
				relPkg:   "",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
					Path:   "/rsc/pdf",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git"},
			},
		},
		// Regression - gh does allow two-letter usernames
		{
			"github.com/kr/pretty",
			&remoteRepo{
				repoRoot: "github.com/kr/pretty",
				relPkg:   "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "kr/pretty",
				},
				Schemes: gitSchemes,
				VCS:     []string{"git"},
			},
		},
	}

	for _, fix := range fixtures {
		got, err := deduceRemoteRepo(fix.path)
		want := fix.want

		if want == nil {
			if err == nil {
				t.Errorf("deduceRemoteRepo(%q): Error expected but not received", fix.path)
			}
			continue
		}

		if err != nil {
			t.Errorf("deduceRemoteRepo(%q): %v", fix.path, err)
			continue
		}

		if got.repoRoot != want.repoRoot {
			t.Errorf("deduceRemoteRepo(%q): Base was %s, wanted %s", fix.path, got.repoRoot, want.repoRoot)
		}
		if got.relPkg != want.relPkg {
			t.Errorf("deduceRemoteRepo(%q): RelPkg was %s, wanted %s", fix.path, got.relPkg, want.relPkg)
		}
		if !reflect.DeepEqual(got.CloneURL, want.CloneURL) {
			// misspelling things is cool when it makes columns line up
			t.Errorf("deduceRemoteRepo(%q): CloneURL disagreement:\n(GOT) %s\n(WNT) %s", fix.path, ufmt(got.CloneURL), ufmt(want.CloneURL))
		}
		if !reflect.DeepEqual(got.VCS, want.VCS) {
			t.Errorf("deduceRemoteRepo(%q): VCS was %s, wanted %s", fix.path, got.VCS, want.VCS)
		}
		if !reflect.DeepEqual(got.Schemes, want.Schemes) {
			t.Errorf("deduceRemoteRepo(%q): Schemes was %s, wanted %s", fix.path, got.Schemes, want.Schemes)
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
