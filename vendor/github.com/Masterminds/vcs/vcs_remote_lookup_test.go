package vcs

import (
	"strings"
	"testing"
)

func TestVCSLookup(t *testing.T) {
	// TODO: Expand to make sure it detected the right vcs.
	urlList := map[string]struct {
		work bool
		t    Type
	}{
		"https://github.com/masterminds":                                   {work: false, t: Git},
		"https://github.com/Masterminds/VCSTestRepo":                       {work: true, t: Git},
		"https://bitbucket.org/mattfarina/testhgrepo":                      {work: true, t: Hg},
		"https://bitbucket.org/mattfarina/repo-does-not-exist":             {work: false, t: Hg},
		"https://bitbucket.org/mattfarina/private-repo-for-vcs-testing":    {work: false, t: Hg},
		"https://launchpad.net/govcstestbzrrepo/trunk":                     {work: true, t: Bzr},
		"https://launchpad.net/~mattfarina/+junk/mygovcstestbzrrepo":       {work: true, t: Bzr},
		"https://launchpad.net/~mattfarina/+junk/mygovcstestbzrrepo/trunk": {work: true, t: Bzr},
		"https://git.launchpad.net/govcstestgitrepo":                       {work: true, t: Git},
		"https://git.launchpad.net/~mattfarina/+git/mygovcstestgitrepo":    {work: true, t: Git},
		"https://hub.jazz.net/git/user1/pkgname":                           {work: true, t: Git},
		"https://hub.jazz.net/git/user1/pkgname/subpkg/subpkg/subpkg":      {work: true, t: Git},
		"https://hubs.jazz.net/git/user1/pkgname":                          {work: false, t: Git},
		"http://farbtastic.googlecode.com/svn/":                            {work: true, t: Svn},
		"http://farbtastic.googlecode.com/svn/trunk":                       {work: true, t: Svn},
		"https://example.com/foo/bar.git":                                  {work: true, t: Git},
		"https://example.com/foo/bar.svn":                                  {work: true, t: Svn},
		"https://example.com/foo/bar/baz.bzr":                              {work: true, t: Bzr},
		"https://example.com/foo/bar/baz.hg":                               {work: true, t: Hg},
		"https://gopkg.in/tomb.v1":                                         {work: true, t: Git},
		"https://golang.org/x/net":                                         {work: true, t: Git},
		"https://speter.net/go/exp/math/dec/inf":                           {work: true, t: Git},
		"git@github.com:Masterminds/vcs.git":                               {work: true, t: Git},
		"git@example.com:foo.git":                                          {work: true, t: Git},
		"ssh://hg@bitbucket.org/mattfarina/testhgrepo":                     {work: true, t: Hg},
		"git@bitbucket.org:mattfarina/glide-bitbucket-example.git":         {work: true, t: Git},
		"git+ssh://example.com/foo/bar":                                    {work: true, t: Git},
		"git://example.com/foo/bar":                                        {work: true, t: Git},
		"bzr+ssh://example.com/foo/bar":                                    {work: true, t: Bzr},
		"svn+ssh://example.com/foo/bar":                                    {work: true, t: Svn},
		"git@example.com:foo/bar":                                          {work: true, t: Git},
		"hg@example.com:foo/bar":                                           {work: true, t: Hg},
	}

	for u, c := range urlList {
		ty, _, err := detectVcsFromRemote(u)
		if err == nil && c.work == false {
			t.Errorf("Error detecting VCS from URL(%s)", u)
		}

		if err == ErrCannotDetectVCS && c.work == true {
			t.Errorf("Error detecting VCS from URL(%s)", u)
		}

		if err != nil && c.work == true {
			t.Errorf("Error detecting VCS from URL(%s): %s", u, err)
		}

		if err != nil &&
			err != ErrCannotDetectVCS &&
			!strings.HasSuffix(err.Error(), "Not Found") &&
			!strings.HasSuffix(err.Error(), "Access Denied") &&
			c.work == false {
			t.Errorf("Unexpected error returned (%s): %s", u, err)
		}

		if c.work == true && ty != c.t {
			t.Errorf("Incorrect VCS type returned(%s)", u)
		}
	}
}

func TestNotFound(t *testing.T) {
	_, _, err := detectVcsFromRemote("https://mattfarina.com/notfound")
	if err == nil || !strings.HasSuffix(err.Error(), " Not Found") {
		t.Errorf("Failed to find not found repo")
	}

	_, err = NewRepo("https://mattfarina.com/notfound", "")
	if err == nil || !strings.HasSuffix(err.Error(), " Not Found") {
		t.Errorf("Failed to find not found repo")
	}
}

func TestAccessDenied(t *testing.T) {
	_, _, err := detectVcsFromRemote("https://bitbucket.org/mattfarina/private-repo-for-vcs-testing")
	if err == nil || err.Error() != "Access Denied" {
		t.Errorf("Failed to detect access denied")
	}

	_, err = NewRepo("https://bitbucket.org/mattfarina/private-repo-for-vcs-testing", "")
	if err == nil || err.Error() != "Access Denied" {
		t.Errorf("Failed to detect access denied")
	}
}
