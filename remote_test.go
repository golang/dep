package vsolver

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
)

func TestDeduceRemotes(t *testing.T) {
	fixtures := []struct {
		path string
		want *remoteRepo
	}{
		{
			"github.com/sdboyer/vsolver",
			&remoteRepo{
				Base:   "github.com/sdboyer/vsolver",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/vsolver",
				},
				Schemes: nil,
				VCS:     []string{"git"},
			},
		},
		{
			"github.com/sdboyer/vsolver/foo",
			&remoteRepo{
				Base:   "github.com/sdboyer/vsolver",
				RelPkg: "foo",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/vsolver",
				},
				Schemes: nil,
				VCS:     []string{"git"},
			},
		},
		{
			"git@github.com:sdboyer/vsolver",
			&remoteRepo{
				Base:   "github.com/sdboyer/vsolver",
				RelPkg: "",
				CloneURL: &url.URL{
					Scheme: "ssh",
					User:   url.User("git"),
					Host:   "github.com",
					Path:   "sdboyer/vsolver",
				},
				Schemes: []string{"ssh"},
				VCS:     []string{"git"},
			},
		},
		{
			"https://github.com/sdboyer/vsolver/foo",
			&remoteRepo{
				Base:   "github.com/sdboyer/vsolver",
				RelPkg: "foo",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
					Path:   "sdboyer/vsolver",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git"},
			},
		},
		{
			"https://github.com/sdboyer/vsolver/foo/bar",
			&remoteRepo{
				Base:   "github.com/sdboyer/vsolver",
				RelPkg: "foo/bar",
				CloneURL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
					Path:   "sdboyer/vsolver",
				},
				Schemes: []string{"https"},
				VCS:     []string{"git"},
			},
		},
		// some invalid github username patterns
		{
			"github.com/-sdboyer/vsolver/foo",
			nil,
		},
		{
			"github.com/sdboyer-/vsolver/foo",
			nil,
		},
		{
			"github.com/sdbo.yer/vsolver/foo",
			nil,
		},
		{
			"github.com/sdbo_yer/vsolver/foo",
			nil,
		},
		{
			"gopkg.in/sdboyer/vsolver.v0",
			&remoteRepo{
				Base:   "gopkg.in/sdboyer/vsolver.v0",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/vsolver",
				},
				VCS: []string{"git"},
			},
		},
		{
			"gopkg.in/sdboyer/vsolver.v0/foo",
			&remoteRepo{
				Base:   "gopkg.in/sdboyer/vsolver.v0",
				RelPkg: "foo",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/vsolver",
				},
				VCS: []string{"git"},
			},
		},
		{
			"gopkg.in/sdboyer/vsolver.v0/foo/bar",
			&remoteRepo{
				Base:   "gopkg.in/sdboyer/vsolver.v0",
				RelPkg: "foo/bar",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "sdboyer/vsolver",
				},
				VCS: []string{"git"},
			},
		},
		{
			"gopkg.in/yaml.v1",
			&remoteRepo{
				Base:   "gopkg.in/yaml.v1",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "go-pkg/yaml",
				},
				VCS: []string{"git"},
			},
		},
		{
			"gopkg.in/yaml.v1/foo/bar",
			&remoteRepo{
				Base:   "gopkg.in/yaml.v1",
				RelPkg: "foo/bar",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "go-pkg/yaml",
				},
				VCS: []string{"git"},
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
				Base:   "hub.jazz.net/git/user1/pkgname",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "hub.jazz.net",
					Path: "git/user1/pkgname",
				},
				VCS: []string{"git"},
			},
		},
		{
			"hub.jazz.net/git/user1/pkgname/submodule/submodule/submodule",
			&remoteRepo{
				Base:   "hub.jazz.net/git/user1/pkgname",
				RelPkg: "submodule/submodule/submodule",
				CloneURL: &url.URL{
					Host: "hub.jazz.net",
					Path: "git/user1/pkgname",
				},
				VCS: []string{"git"},
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
				Base:   "hub.jazz.net/git/user/pkg.name",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "hub.jazz.net",
					Path: "git/user/pkg.name",
				},
				VCS: []string{"git"},
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
				Base:   "bitbucket.org/sdboyer/reporoot",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "bitbucket.org",
					Path: "sdboyer/reporoot",
				},
				VCS: []string{"git", "hg"},
			},
		},
		{
			"bitbucket.org/sdboyer/reporoot/foo/bar",
			&remoteRepo{
				Base:   "bitbucket.org/sdboyer/reporoot",
				RelPkg: "foo/bar",
				CloneURL: &url.URL{
					Host: "bitbucket.org",
					Path: "sdboyer/reporoot",
				},
				VCS: []string{"git", "hg"},
			},
		},
		{
			"https://bitbucket.org/sdboyer/reporoot/foo/bar",
			&remoteRepo{
				Base:   "bitbucket.org/sdboyer/reporoot",
				RelPkg: "foo/bar",
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
				Base:   "launchpad.net/govcstestbzrrepo",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "launchpad.net",
					Path: "govcstestbzrrepo",
				},
				VCS: []string{"bzr"},
			},
		},
		{
			"launchpad.net/govcstestbzrrepo/foo/bar",
			&remoteRepo{
				Base:   "launchpad.net/govcstestbzrrepo",
				RelPkg: "foo/bar",
				CloneURL: &url.URL{
					Host: "launchpad.net",
					Path: "govcstestbzrrepo",
				},
				VCS: []string{"bzr"},
			},
		},
		{
			"launchpad.net/repo root",
			nil,
		},
		{
			"git.launchpad.net/reporoot",
			&remoteRepo{
				Base:   "git.launchpad.net/reporoot",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "git.launchpad.net",
					Path: "reporoot",
				},
				VCS: []string{"git"},
			},
		},
		{
			"git.launchpad.net/reporoot/foo/bar",
			&remoteRepo{
				Base:   "git.launchpad.net/reporoot",
				RelPkg: "foo/bar",
				CloneURL: &url.URL{
					Host: "git.launchpad.net",
					Path: "reporoot",
				},
				VCS: []string{"git"},
			},
		},
		{
			"git.launchpad.net/reporoot",
			&remoteRepo{
				Base:   "git.launchpad.net/reporoot",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "git.launchpad.net",
					Path: "reporoot",
				},
				VCS: []string{"git"},
			},
		},
		{
			"git.launchpad.net/repo root",
			nil,
		},
		{
			"git.apache.org/package-name.git",
			&remoteRepo{
				Base:   "git.apache.org/package-name.git",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "git.apache.org",
					Path: "package-name.git",
				},
				VCS: []string{"git"},
			},
		},
		{
			"git.apache.org/package-name.git/foo/bar",
			&remoteRepo{
				Base:   "git.apache.org/package-name.git",
				RelPkg: "foo/bar",
				CloneURL: &url.URL{
					Host: "git.apache.org",
					Path: "package-name.git",
				},
				VCS: []string{"git"},
			},
		},
		// Regression - gh does allow 2-letter usernames
		{
			"github.com/kr/pretty",
			&remoteRepo{
				Base:   "github.com/kr/pretty",
				RelPkg: "",
				CloneURL: &url.URL{
					Host: "github.com",
					Path: "kr/pretty",
				},
				Schemes: nil,
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

		if got.Base != want.Base {
			t.Errorf("deduceRemoteRepo(%q): Base was %s, wanted %s", fix.path, got.Base, want.Base)
		}
		if got.RelPkg != want.RelPkg {
			t.Errorf("deduceRemoteRepo(%q): RelPkg was %s, wanted %s", fix.path, got.RelPkg, want.RelPkg)
		}
		if !reflect.DeepEqual(got.CloneURL, want.CloneURL) {
			// mispelling things is cool when it makes columns line up
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
