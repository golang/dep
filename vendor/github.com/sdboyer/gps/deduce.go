package gps

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var (
	gitSchemes = []string{"https", "ssh", "git", "http"}
	bzrSchemes = []string{"https", "bzr+ssh", "bzr", "http"}
	hgSchemes  = []string{"https", "ssh", "http"}
	svnSchemes = []string{"https", "http", "svn", "svn+ssh"}
)

func validateVCSScheme(scheme, typ string) bool {
	// everything allows plain ssh
	if scheme == "ssh" {
		return true
	}

	var schemes []string
	switch typ {
	case "git":
		schemes = gitSchemes
	case "bzr":
		schemes = bzrSchemes
	case "hg":
		schemes = hgSchemes
	case "svn":
		schemes = svnSchemes
	default:
		panic(fmt.Sprint("unsupported vcs type", scheme))
	}

	for _, valid := range schemes {
		if scheme == valid {
			return true
		}
	}
	return false
}

// Regexes for the different known import path flavors
var (
	// This regex allowed some usernames that github currently disallows. They
	// may have allowed them in the past; keeping it in case we need to revert.
	//ghRegex      = regexp.MustCompile(`^(?P<root>github\.com/([A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+))(/[A-Za-z0-9_.\-]+)*$`)
	ghRegex      = regexp.MustCompile(`^(?P<root>github\.com(/[A-Za-z0-9][-A-Za-z0-9]*[A-Za-z0-9]/[A-Za-z0-9_.\-]+))((?:/[A-Za-z0-9_.\-]+)*)$`)
	gpinNewRegex = regexp.MustCompile(`^(?P<root>gopkg\.in(?:(/[a-zA-Z0-9][-a-zA-Z0-9]+)?)(/[a-zA-Z][-.a-zA-Z0-9]*)\.((?:v0|v[1-9][0-9]*)(?:\.0|\.[1-9][0-9]*){0,2}(?:-unstable)?)(?:\.git)?)((?:/[a-zA-Z0-9][-.a-zA-Z0-9]*)*)$`)
	//gpinOldRegex = regexp.MustCompile(`^(?P<root>gopkg\.in/(?:([a-z0-9][-a-z0-9]+)/)?((?:v0|v[1-9][0-9]*)(?:\.0|\.[1-9][0-9]*){0,2}(-unstable)?)/([a-zA-Z][-a-zA-Z0-9]*)(?:\.git)?)((?:/[a-zA-Z][-a-zA-Z0-9]*)*)$`)
	bbRegex = regexp.MustCompile(`^(?P<root>bitbucket\.org(?P<bitname>/[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+))((?:/[A-Za-z0-9_.\-]+)*)$`)
	//lpRegex = regexp.MustCompile(`^(?P<root>launchpad\.net/([A-Za-z0-9-._]+)(/[A-Za-z0-9-._]+)?)(/.+)?`)
	lpRegex = regexp.MustCompile(`^(?P<root>launchpad\.net(/[A-Za-z0-9-._]+))((?:/[A-Za-z0-9_.\-]+)*)?`)
	//glpRegex = regexp.MustCompile(`^(?P<root>git\.launchpad\.net/([A-Za-z0-9_.\-]+)|~[A-Za-z0-9_.\-]+/(\+git|[A-Za-z0-9_.\-]+)/[A-Za-z0-9_.\-]+)$`)
	glpRegex = regexp.MustCompile(`^(?P<root>git\.launchpad\.net(/[A-Za-z0-9_.\-]+))((?:/[A-Za-z0-9_.\-]+)*)$`)
	//gcRegex      = regexp.MustCompile(`^(?P<root>code\.google\.com/[pr]/(?P<project>[a-z0-9\-]+)(\.(?P<subrepo>[a-z0-9\-]+))?)(/[A-Za-z0-9_.\-]+)*$`)
	jazzRegex         = regexp.MustCompile(`^(?P<root>hub\.jazz\.net(/git/[a-z0-9]+/[A-Za-z0-9_.\-]+))((?:/[A-Za-z0-9_.\-]+)*)$`)
	apacheRegex       = regexp.MustCompile(`^(?P<root>git\.apache\.org(/[a-z0-9_.\-]+\.git))((?:/[A-Za-z0-9_.\-]+)*)$`)
	vcsExtensionRegex = regexp.MustCompile(`^(?P<root>([a-z0-9.\-]+\.)+[a-z0-9.\-]+(:[0-9]+)?/[A-Za-z0-9_.\-/~]*?\.(?P<vcs>bzr|git|hg|svn))((?:/[A-Za-z0-9_.\-]+)*)$`)
)

// Other helper regexes
var (
	scpSyntaxRe = regexp.MustCompile(`^([a-zA-Z0-9_]+)@([a-zA-Z0-9._-]+):(.*)$`)
	pathvld     = regexp.MustCompile(`^([A-Za-z0-9-]+)(\.[A-Za-z0-9-]+)+(/[A-Za-z0-9-_.~]+)*$`)
)

func pathDeducerTrie() deducerTrie {
	dxt := newDeducerTrie()

	dxt.Insert("github.com/", githubDeducer{regexp: ghRegex})
	dxt.Insert("gopkg.in/", gopkginDeducer{regexp: gpinNewRegex})
	dxt.Insert("bitbucket.org/", bitbucketDeducer{regexp: bbRegex})
	dxt.Insert("launchpad.net/", launchpadDeducer{regexp: lpRegex})
	dxt.Insert("git.launchpad.net/", launchpadGitDeducer{regexp: glpRegex})
	dxt.Insert("hub.jazz.net/", jazzDeducer{regexp: jazzRegex})
	dxt.Insert("git.apache.org/", apacheDeducer{regexp: apacheRegex})

	return dxt
}

type pathDeducer interface {
	deduceRoot(string) (string, error)
	deduceSource(string, *url.URL) (maybeSource, error)
}

type githubDeducer struct {
	regexp *regexp.Regexp
}

func (m githubDeducer) deduceRoot(path string) (string, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return "", fmt.Errorf("%s is not a valid path for a source on github.com", path)
	}

	return "github.com" + v[2], nil
}

func (m githubDeducer) deduceSource(path string, u *url.URL) (maybeSource, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return nil, fmt.Errorf("%s is not a valid path for a source on github.com", path)
	}

	u.Host = "github.com"
	u.Path = v[2]

	if u.Scheme == "ssh" && u.User != nil && u.User.Username() != "git" {
		return nil, fmt.Errorf("github ssh must be accessed via the 'git' user; %s was provided", u.User.Username())
	} else if u.Scheme != "" {
		if !validateVCSScheme(u.Scheme, "git") {
			return nil, fmt.Errorf("%s is not a valid scheme for accessing a git repository", u.Scheme)
		}
		if u.Scheme == "ssh" {
			u.User = url.User("git")
		}
		return maybeGitSource{url: u}, nil
	}

	mb := make(maybeSources, len(gitSchemes))
	for k, scheme := range gitSchemes {
		u2 := *u
		if scheme == "ssh" {
			u2.User = url.User("git")
		}
		u2.Scheme = scheme
		mb[k] = maybeGitSource{url: &u2}
	}

	return mb, nil
}

type bitbucketDeducer struct {
	regexp *regexp.Regexp
}

func (m bitbucketDeducer) deduceRoot(path string) (string, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return "", fmt.Errorf("%s is not a valid path for a source on bitbucket.org", path)
	}

	return "bitbucket.org" + v[2], nil
}

func (m bitbucketDeducer) deduceSource(path string, u *url.URL) (maybeSource, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return nil, fmt.Errorf("%s is not a valid path for a source on bitbucket.org", path)
	}

	u.Host = "bitbucket.org"
	u.Path = v[2]

	// This isn't definitive, but it'll probably catch most
	isgit := strings.HasSuffix(u.Path, ".git") || (u.User != nil && u.User.Username() == "git")
	ishg := strings.HasSuffix(u.Path, ".hg") || (u.User != nil && u.User.Username() == "hg")

	// TODO(sdboyer) resolve scm ambiguity if needed by querying bitbucket's REST API
	if u.Scheme != "" {
		validgit, validhg := validateVCSScheme(u.Scheme, "git"), validateVCSScheme(u.Scheme, "hg")
		if isgit {
			if !validgit {
				// This is unreachable for now, as the git schemes are a
				// superset of the hg schemes
				return nil, fmt.Errorf("%s is not a valid scheme for accessing a git repository", u.Scheme)
			}
			return maybeGitSource{url: u}, nil
		} else if ishg {
			if !validhg {
				return nil, fmt.Errorf("%s is not a valid scheme for accessing an hg repository", u.Scheme)
			}
			return maybeHgSource{url: u}, nil
		} else if !validgit && !validhg {
			return nil, fmt.Errorf("%s is not a valid scheme for accessing either a git or hg repository", u.Scheme)
		}

		// No other choice, make an option for both git and hg
		return maybeSources{
			maybeHgSource{url: u},
			maybeGitSource{url: u},
		}, nil
	}

	mb := make(maybeSources, 0)
	// git is probably more common, even on bitbucket. however, bitbucket
	// appears to fail _extremely_ slowly on git pings (ls-remote) when the
	// underlying repository is actually an hg repository, so it's better
	// to try hg first.
	if !isgit {
		for _, scheme := range hgSchemes {
			u2 := *u
			if scheme == "ssh" {
				u2.User = url.User("hg")
			}
			u2.Scheme = scheme
			mb = append(mb, maybeHgSource{url: &u2})
		}
	}

	if !ishg {
		for _, scheme := range gitSchemes {
			u2 := *u
			if scheme == "ssh" {
				u2.User = url.User("git")
			}
			u2.Scheme = scheme
			mb = append(mb, maybeGitSource{url: &u2})
		}
	}

	return mb, nil
}

type gopkginDeducer struct {
	regexp *regexp.Regexp
}

func (m gopkginDeducer) deduceRoot(p string) (string, error) {
	v, err := m.parseAndValidatePath(p)
	if err != nil {
		return "", err
	}

	return v[1], nil
}

func (m gopkginDeducer) parseAndValidatePath(p string) ([]string, error) {
	v := m.regexp.FindStringSubmatch(p)
	if v == nil {
		return nil, fmt.Errorf("%s is not a valid path for a source on gopkg.in", p)
	}

	// We duplicate some logic from the gopkg.in server in order to validate the
	// import path string without having to make a network request
	if strings.Contains(v[4], ".") {
		return nil, fmt.Errorf("%s is not a valid import path; gopkg.in only allows major versions (%q instead of %q)",
			p, v[4][:strings.Index(v[4], ".")], v[4])
	}

	return v, nil
}

func (m gopkginDeducer) deduceSource(p string, u *url.URL) (maybeSource, error) {
	// Reuse root detection logic for initial validation
	v, err := m.parseAndValidatePath(p)
	if err != nil {
		return nil, err
	}

	// Putting a scheme on gopkg.in would be really weird, disallow it
	if u.Scheme != "" {
		return nil, fmt.Errorf("Specifying alternate schemes on gopkg.in imports is not permitted")
	}

	// gopkg.in is always backed by github
	u.Host = "github.com"
	if v[2] == "" {
		elem := v[3][1:]
		u.Path = path.Join("/go-"+elem, elem)
	} else {
		u.Path = path.Join(v[2], v[3])
	}

	mb := make(maybeSources, len(gitSchemes))
	for k, scheme := range gitSchemes {
		u2 := *u
		if scheme == "ssh" {
			u2.User = url.User("git")
		}
		u2.Scheme = scheme
		mb[k] = maybeGitSource{url: &u2}
	}

	return mb, nil
}

type launchpadDeducer struct {
	regexp *regexp.Regexp
}

func (m launchpadDeducer) deduceRoot(path string) (string, error) {
	// TODO(sdboyer) lp handling is nasty - there's ambiguities which can only really
	// be resolved with a metadata request. See https://github.com/golang/go/issues/11436
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return "", fmt.Errorf("%s is not a valid path for a source on launchpad.net", path)
	}

	return "launchpad.net" + v[2], nil
}

func (m launchpadDeducer) deduceSource(path string, u *url.URL) (maybeSource, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return nil, fmt.Errorf("%s is not a valid path for a source on launchpad.net", path)
	}

	u.Host = "launchpad.net"
	u.Path = v[2]

	if u.Scheme != "" {
		if !validateVCSScheme(u.Scheme, "bzr") {
			return nil, fmt.Errorf("%s is not a valid scheme for accessing a bzr repository", u.Scheme)
		}
		return maybeBzrSource{url: u}, nil
	}

	mb := make(maybeSources, len(bzrSchemes))
	for k, scheme := range bzrSchemes {
		u2 := *u
		u2.Scheme = scheme
		mb[k] = maybeBzrSource{url: &u2}
	}

	return mb, nil
}

type launchpadGitDeducer struct {
	regexp *regexp.Regexp
}

func (m launchpadGitDeducer) deduceRoot(path string) (string, error) {
	// TODO(sdboyer) same ambiguity issues as with normal bzr lp
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return "", fmt.Errorf("%s is not a valid path for a source on git.launchpad.net", path)
	}

	return "git.launchpad.net" + v[2], nil
}

func (m launchpadGitDeducer) deduceSource(path string, u *url.URL) (maybeSource, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return nil, fmt.Errorf("%s is not a valid path for a source on git.launchpad.net", path)
	}

	u.Host = "git.launchpad.net"
	u.Path = v[2]

	if u.Scheme != "" {
		if !validateVCSScheme(u.Scheme, "git") {
			return nil, fmt.Errorf("%s is not a valid scheme for accessing a git repository", u.Scheme)
		}
		return maybeGitSource{url: u}, nil
	}

	mb := make(maybeSources, len(gitSchemes))
	for k, scheme := range gitSchemes {
		u2 := *u
		u2.Scheme = scheme
		mb[k] = maybeGitSource{url: &u2}
	}

	return mb, nil
}

type jazzDeducer struct {
	regexp *regexp.Regexp
}

func (m jazzDeducer) deduceRoot(path string) (string, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return "", fmt.Errorf("%s is not a valid path for a source on hub.jazz.net", path)
	}

	return "hub.jazz.net" + v[2], nil
}

func (m jazzDeducer) deduceSource(path string, u *url.URL) (maybeSource, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return nil, fmt.Errorf("%s is not a valid path for a source on hub.jazz.net", path)
	}

	u.Host = "hub.jazz.net"
	u.Path = v[2]

	switch u.Scheme {
	case "":
		u.Scheme = "https"
		fallthrough
	case "https":
		return maybeGitSource{url: u}, nil
	default:
		return nil, fmt.Errorf("IBM's jazz hub only supports https, %s is not allowed", u.String())
	}
}

type apacheDeducer struct {
	regexp *regexp.Regexp
}

func (m apacheDeducer) deduceRoot(path string) (string, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return "", fmt.Errorf("%s is not a valid path for a source on git.apache.org", path)
	}

	return "git.apache.org" + v[2], nil
}

func (m apacheDeducer) deduceSource(path string, u *url.URL) (maybeSource, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return nil, fmt.Errorf("%s is not a valid path for a source on git.apache.org", path)
	}

	u.Host = "git.apache.org"
	u.Path = v[2]

	if u.Scheme != "" {
		if !validateVCSScheme(u.Scheme, "git") {
			return nil, fmt.Errorf("%s is not a valid scheme for accessing a git repository", u.Scheme)
		}
		return maybeGitSource{url: u}, nil
	}

	mb := make(maybeSources, len(gitSchemes))
	for k, scheme := range gitSchemes {
		u2 := *u
		u2.Scheme = scheme
		mb[k] = maybeGitSource{url: &u2}
	}

	return mb, nil
}

type vcsExtensionDeducer struct {
	regexp *regexp.Regexp
}

func (m vcsExtensionDeducer) deduceRoot(path string) (string, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return "", fmt.Errorf("%s contains no vcs extension hints for matching", path)
	}

	return v[1], nil
}

func (m vcsExtensionDeducer) deduceSource(path string, u *url.URL) (maybeSource, error) {
	v := m.regexp.FindStringSubmatch(path)
	if v == nil {
		return nil, fmt.Errorf("%s contains no vcs extension hints for matching", path)
	}

	switch v[4] {
	case "git", "hg", "bzr":
		x := strings.SplitN(v[1], "/", 2)
		// TODO(sdboyer) is this actually correct for bzr?
		u.Host = x[0]
		u.Path = "/" + x[1]

		if u.Scheme != "" {
			if !validateVCSScheme(u.Scheme, v[4]) {
				return nil, fmt.Errorf("%s is not a valid scheme for accessing %s repositories (path %s)", u.Scheme, v[4], path)
			}

			switch v[4] {
			case "git":
				return maybeGitSource{url: u}, nil
			case "bzr":
				return maybeBzrSource{url: u}, nil
			case "hg":
				return maybeHgSource{url: u}, nil
			}
		}

		var schemes []string
		var mb maybeSources
		var f func(k int, u *url.URL)

		switch v[4] {
		case "git":
			schemes = gitSchemes
			f = func(k int, u *url.URL) {
				mb[k] = maybeGitSource{url: u}
			}
		case "bzr":
			schemes = bzrSchemes
			f = func(k int, u *url.URL) {
				mb[k] = maybeBzrSource{url: u}
			}
		case "hg":
			schemes = hgSchemes
			f = func(k int, u *url.URL) {
				mb[k] = maybeHgSource{url: u}
			}
		}

		mb = make(maybeSources, len(schemes))
		for k, scheme := range schemes {
			u2 := *u
			u2.Scheme = scheme
			f(k, &u2)
		}

		return mb, nil
	default:
		return nil, fmt.Errorf("unknown repository type: %q", v[4])
	}
}

type stringFuture func() (string, error)
type sourceFuture func() (source, string, error)
type partialSourceFuture func(string, ProjectAnalyzer) sourceFuture

type deductionFuture struct {
	// rslow indicates that the root future may be a slow call (that it has to
	// hit the network for some reason)
	rslow bool
	root  stringFuture
	psf   partialSourceFuture
}

// deduceFromPath takes an import path and attempts to deduce various
// metadata about it - what type of source should handle it, and where its
// "root" is (for vcs repositories, the repository root).
//
// The results are wrapped in futures, as most of these operations require at
// least some network activity to complete. For the first return value, network
// activity will be triggered when the future is called. For the second,
// network activity is triggered only when calling the sourceFuture returned
// from the partialSourceFuture.
func (sm *SourceMgr) deduceFromPath(path string) (deductionFuture, error) {
	opath := path
	u, path, err := normalizeURI(path)
	if err != nil {
		return deductionFuture{}, err
	}

	// Helpers to futurize the results from deducers
	strfut := func(s string) stringFuture {
		return func() (string, error) {
			return s, nil
		}
	}

	srcfut := func(mb maybeSource) partialSourceFuture {
		return func(cachedir string, an ProjectAnalyzer) sourceFuture {
			var src source
			var ident string
			var err error

			c := make(chan struct{}, 1)
			go func() {
				defer close(c)
				src, ident, err = mb.try(cachedir, an)
			}()

			return func() (source, string, error) {
				<-c
				return src, ident, err
			}
		}
	}

	// First, try the root path-based matches
	if _, mtchi, has := sm.dxt.LongestPrefix(path); has {
		mtch := mtchi.(pathDeducer)
		root, err := mtch.deduceRoot(path)
		if err != nil {
			return deductionFuture{}, err
		}
		mb, err := mtch.deduceSource(path, u)
		if err != nil {
			return deductionFuture{}, err
		}

		return deductionFuture{
			rslow: false,
			root:  strfut(root),
			psf:   srcfut(mb),
		}, nil
	}

	// Next, try the vcs extension-based (infix) matcher
	exm := vcsExtensionDeducer{regexp: vcsExtensionRegex}
	if root, err := exm.deduceRoot(path); err == nil {
		mb, err := exm.deduceSource(path, u)
		if err != nil {
			return deductionFuture{}, err
		}

		return deductionFuture{
			rslow: false,
			root:  strfut(root),
			psf:   srcfut(mb),
		}, nil
	}

	// No luck so far. maybe it's one of them vanity imports?
	// We have to get a little fancier for the metadata lookup by chaining the
	// source future onto the metadata future

	// Declare these out here so they're available for the source future
	var vcs string
	var ru *url.URL

	// Kick off the vanity metadata fetch
	var importroot string
	var futerr error
	c := make(chan struct{}, 1)
	go func() {
		defer close(c)
		var reporoot string
		importroot, vcs, reporoot, futerr = parseMetadata(path)
		if futerr != nil {
			futerr = fmt.Errorf("unable to deduce repository and source type for: %q", opath)
			return
		}

		// If we got something back at all, then it supercedes the actual input for
		// the real URL to hit
		ru, futerr = url.Parse(reporoot)
		if futerr != nil {
			futerr = fmt.Errorf("server returned bad URL when searching for vanity import: %q", reporoot)
			importroot = ""
			return
		}
	}()

	// Set up the root func to catch the result
	root := func() (string, error) {
		<-c
		return importroot, futerr
	}

	src := func(cachedir string, an ProjectAnalyzer) sourceFuture {
		var src source
		var ident string
		var err error

		c := make(chan struct{}, 1)
		go func() {
			defer close(c)
			// make sure the metadata future is finished (without errors), thus
			// guaranteeing that ru and vcs will be populated
			_, err := root()
			if err != nil {
				return
			}
			ident = ru.String()

			var m maybeSource
			switch vcs {
			case "git":
				m = maybeGitSource{url: ru}
			case "bzr":
				m = maybeBzrSource{url: ru}
			case "hg":
				m = maybeHgSource{url: ru}
			}

			if m != nil {
				src, ident, err = m.try(cachedir, an)
			} else {
				err = fmt.Errorf("unsupported vcs type %s", vcs)
			}
		}()

		return func() (source, string, error) {
			<-c
			return src, ident, err
		}
	}

	return deductionFuture{
		rslow: true,
		root:  root,
		psf:   src,
	}, nil
}

func normalizeURI(p string) (u *url.URL, newpath string, err error) {
	if m := scpSyntaxRe.FindStringSubmatch(p); m != nil {
		// Match SCP-like syntax and convert it to a URL.
		// Eg, "git@github.com:user/repo" becomes
		// "ssh://git@github.com/user/repo".
		u = &url.URL{
			Scheme: "ssh",
			User:   url.User(m[1]),
			Host:   m[2],
			Path:   "/" + m[3],
			// TODO(sdboyer) This is what stdlib sets; grok why better
			//RawPath: m[3],
		}
	} else {
		u, err = url.Parse(p)
		if err != nil {
			return nil, "", fmt.Errorf("%q is not a valid URI", p)
		}
	}

	// If no scheme was passed, then the entire path will have been put into
	// u.Path. Either way, construct the normalized path correctly.
	if u.Host == "" {
		newpath = p
	} else {
		newpath = path.Join(u.Host, u.Path)
	}

	if !pathvld.MatchString(newpath) {
		return nil, "", fmt.Errorf("%q is not a valid import path", newpath)
	}

	return
}

// fetchMetadata fetches the remote metadata for path.
func fetchMetadata(path string) (rc io.ReadCloser, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to determine remote metadata protocol: %s", err)
		}
	}()

	// try https first
	rc, err = doFetchMetadata("https", path)
	if err == nil {
		return
	}

	rc, err = doFetchMetadata("http", path)
	return
}

func doFetchMetadata(scheme, path string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s://%s?go-get=1", scheme, path)
	switch scheme {
	case "https", "http":
		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to access url %q", url)
		}
		return resp.Body, nil
	default:
		return nil, fmt.Errorf("unknown remote protocol scheme: %q", scheme)
	}
}

// parseMetadata fetches and decodes remote metadata for path.
func parseMetadata(path string) (string, string, string, error) {
	rc, err := fetchMetadata(path)
	if err != nil {
		return "", "", "", err
	}
	defer rc.Close()

	imports, err := parseMetaGoImports(rc)
	if err != nil {
		return "", "", "", err
	}
	match := -1
	for i, im := range imports {
		if !strings.HasPrefix(path, im.Prefix) {
			continue
		}
		if match != -1 {
			return "", "", "", fmt.Errorf("multiple meta tags match import path %q", path)
		}
		match = i
	}
	if match == -1 {
		return "", "", "", fmt.Errorf("go-import metadata not found")
	}
	return imports[match].Prefix, imports[match].VCS, imports[match].RepoRoot, nil
}
