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
	}

	for _, fix := range fixtures {
		got, err := deduceRemoteRepo(fix.path)
		want := fix.want

		if want == nil {
			if err == nil {
				t.Errorf("deduceRemoteRepo(%q): Error expected but not received", fix.path)
			} else if testing.Verbose() {
				t.Logf("deduceRemoteRepo(%q) expected err: %v", fix.path, err)
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
