package gps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	radix "github.com/armon/go-radix"
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
	// This regex allows some usernames that github currently disallows. They
	// have allowed them in the past.
	ghRegex      = regexp.MustCompile(`^(?P<root>github\.com(/[A-Za-z0-9][-A-Za-z0-9]*/[A-Za-z0-9_.\-]+))((?:/[A-Za-z0-9_.\-]+)*)$`)
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

func pathDeducerTrie() *deducerTrie {
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
		return nil, fmt.Errorf("specifying alternate schemes on gopkg.in imports is not permitted")
	}

	// gopkg.in is always backed by github
	u.Host = "github.com"
	if v[2] == "" {
		elem := v[3][1:]
		u.Path = path.Join("/go-"+elem, elem)
	} else {
		u.Path = path.Join(v[2], v[3])
	}
	major, err := strconv.ParseUint(v[4][1:], 10, 64)
	if err != nil {
		// this should only be reachable if there's an error in the regex
		return nil, fmt.Errorf("could not parse %q as a gopkg.in major version", v[4][1:])
	}

	mb := make(maybeSources, len(gitSchemes))
	for k, scheme := range gitSchemes {
		u2 := *u
		if scheme == "ssh" {
			u2.User = url.User("git")
		}
		u2.Scheme = scheme
		mb[k] = maybeGopkginSource{
			opath: v[1],
			url:   &u2,
			major: major,
		}
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
				src, ident, err = mb.try(context.TODO(), cachedir, an)
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
		importroot, vcs, reporoot, futerr = parseMetadata(context.Background(), path)
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
			_, err = root()
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
				src, ident, err = m.try(context.TODO(), cachedir, an)
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

type deductionCoordinator struct {
	ctx      context.Context
	callMgr  *callManager
	rootxt   *radix.Tree
	deducext *deducerTrie
	action   chan func()
}

func newDeductionCoordinator(cm *callManager) *deductionCoordinator {
	dc := &deductionCoordinator{
		callMgr:  cm,
		ctx:      cm.getLifetimeContext(),
		rootxt:   radix.New(),
		deducext: pathDeducerTrie(),
	}

	// Start listener loop
	go func() {
		for {
			select {
			case <-dc.ctx.Done():
				close(dc.action)
			case action := <-dc.action:
				action()
			}
		}
	}()

	return dc
}

func (dc *deductionCoordinator) deduceRootPath(path string) (pathDeduction, error) {
	if dc.ctx.Err() != nil {
		return pathDeduction{}, errors.New("deductionCoordinator has been terminated")
	}

	retchan, errchan := make(chan pathDeduction), make(chan error)
	dc.action <- func() {
		hmdDeduce := func(hmd *httpMetadataDeducer) {
			pd, err := hmd.deduce(context.TODO(), path)
			if err != nil {
				errchan <- err
			} else {
				retchan <- pd
			}
		}

		// First, check the rootxt to see if there's a prefix match - if so, we
		// can return that and move on.
		if prefix, data, has := dc.rootxt.LongestPrefix(path); has && isPathPrefixOrEqual(prefix, path) {
			switch d := data.(type) {
			case maybeSource:
				retchan <- pathDeduction{root: prefix, mb: d}
			case *httpMetadataDeducer:
				// Multiple calls have come in for a similar path shape during
				// the window in which the HTTP request to retrieve go get
				// metadata is in flight. Fold this request in with the existing
				// one(s) by giving it its own goroutine that awaits a response
				// from the running httpMetadataDeducer.
				go hmdDeduce(d)
			default:
				panic(fmt.Sprintf("unexpected %T in deductionCoordinator.rootxt: %v", d, d))
			}

			// Finding either a finished maybeSource or an in-flight vanity
			// deduction means there's nothing more to do on this action.
			return
		}

		// No match. Try known path deduction first.
		pd, err := dc.deduceKnownPaths(path)
		if err == nil {
			// Deduction worked; store it in the rootxt, send on retchan and
			// terminate.
			// FIXME(sdboyer) deal with changing path vs. root. Probably needs
			// to be predeclared and reused in the hmd returnFunc
			dc.rootxt.Insert(pd.root, pd.mb)
			retchan <- pd
			return
		}

		if err != errNoKnownPathMatch {
			errchan <- err
			return
		}

		// The err indicates no known path matched. It's still possible that
		// retrieving go get metadata might do the trick.
		hmd := &httpMetadataDeducer{
			basePath: path,
			callMgr:  dc.callMgr,
			// The vanity deducer will call this func with a completed
			// pathDeduction if it succeeds in finding one. We process it
			// back through the action channel to ensure serialized
			// access to the rootxt map.
			returnFunc: func(pd pathDeduction) {
				dc.action <- func() {
					if pd.root != path {
						// Replace the vanity deducer with a real result set, so
						// that subsequent deductions don't hit the network
						// again.
						dc.rootxt.Insert(path, pd.mb)
					}
					dc.rootxt.Insert(pd.root, pd.mb)
				}
			},
		}

		// Save the hmd in the rootxt so that calls checking on similar
		// paths made while the request is in flight can be folded together.
		dc.rootxt.Insert(path, hmd)
		// Spawn a new goroutine for the HTTP-backed deduction process.
		go hmdDeduce(hmd)

	}

	select {
	case pd := <-retchan:
		return pd, nil
	case err := <-errchan:
		return pathDeduction{}, err
	}
}

// pathDeduction represents the results of a successful import path deduction -
// a root path, plus a maybeSource that can be used to attempt to connect to
// the source.
type pathDeduction struct {
	root string
	mb   maybeSource
}

var errNoKnownPathMatch = errors.New("no known path match")

func (dc *deductionCoordinator) deduceKnownPaths(path string) (pathDeduction, error) {
	u, path, err := normalizeURI(path)
	if err != nil {
		return pathDeduction{}, err
	}

	// First, try the root path-based matches
	if _, mtch, has := dc.deducext.LongestPrefix(path); has {
		root, err := mtch.deduceRoot(path)
		if err != nil {
			return pathDeduction{}, err
		}
		mb, err := mtch.deduceSource(path, u)
		if err != nil {
			return pathDeduction{}, err
		}

		return pathDeduction{
			root: root,
			mb:   mb,
		}, nil
	}

	// Next, try the vcs extension-based (infix) matcher
	exm := vcsExtensionDeducer{regexp: vcsExtensionRegex}
	if root, err := exm.deduceRoot(path); err == nil {
		mb, err := exm.deduceSource(path, u)
		if err != nil {
			return pathDeduction{}, err
		}

		return pathDeduction{
			root: root,
			mb:   mb,
		}, nil
	}

	return pathDeduction{}, errNoKnownPathMatch
}

type httpMetadataDeducer struct {
	once       sync.Once
	deduced    pathDeduction
	deduceErr  error
	basePath   string
	returnFunc func(pathDeduction)
	callMgr    *callManager
}

func (hmd *httpMetadataDeducer) deduce(ctx context.Context, path string) (pathDeduction, error) {
	// TODO(sdboyer) can this be replaced by the code in golang.org/x?
	hmd.once.Do(func() {
		ctx, doneFunc, err := hmd.callMgr.setUpCall(ctx, path, ctHTTPMetadata)
		if err != nil {
			hmd.deduceErr = err
			return
		}
		defer doneFunc()

		opath := path
		// FIXME should we need this first return val?
		_, path, err := normalizeURI(path)
		if err != nil {
			hmd.deduceErr = err
			return
		}

		pd := pathDeduction{}

		// Make the HTTP call to attempt to retrieve go-get metadata
		root, vcs, reporoot, err := parseMetadata(ctx, path)
		if err != nil {
			hmd.deduceErr = fmt.Errorf("unable to deduce repository and source type for: %q", opath)
			return
		}
		pd.root = root

		// If we got something back at all, then it supercedes the actual input for
		// the real URL to hit
		repoURL, err := url.Parse(reporoot)
		if err != nil {
			hmd.deduceErr = fmt.Errorf("server returned bad URL when searching for vanity import: %q", reporoot)
			return
		}

		switch vcs {
		case "git":
			pd.mb = maybeGitSource{url: repoURL}
		case "bzr":
			pd.mb = maybeBzrSource{url: repoURL}
		case "hg":
			pd.mb = maybeHgSource{url: repoURL}
		default:
			hmd.deduceErr = fmt.Errorf("unsupported vcs type %s in go-get metadata from %s", vcs, path)
			return
		}

		hmd.deduced = pd
		// All data is assigned for other goroutines that may be waiting. Now,
		// send the pathDeduction back to the deductionCoordinator by calling
		// the returnFunc. This will also remove the reference to this hmd in
		// the coordinator's trie.
		//
		// When this call finishes, it is guaranteed the coordinator will have
		// at least begun running the action to insert the path deduction, which
		// means no other deduction request will be able to interleave and
		// request the same path before the pathDeduction can be processed, but
		// after this hmd has been dereferenced from the trie.
		hmd.returnFunc(pd)
	})

	return hmd.deduced, hmd.deduceErr
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
func fetchMetadata(ctx context.Context, path string) (rc io.ReadCloser, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("unable to determine remote metadata protocol: %s", err)
		}
	}()

	// try https first
	rc, err = doFetchMetadata(ctx, "https", path)
	if err == nil {
		return
	}

	rc, err = doFetchMetadata(ctx, "http", path)
	return
}

func doFetchMetadata(ctx context.Context, scheme, path string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s://%s?go-get=1", scheme, path)
	switch scheme {
	case "https", "http":
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to access url %q", url)
		}

		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to access url %q", url)
		}

		return resp.Body, nil
	default:
		return nil, fmt.Errorf("unknown remote protocol scheme: %q", scheme)
	}
}

// parseMetadata fetches and decodes remote metadata for path.
func parseMetadata(ctx context.Context, path string) (string, string, string, error) {
	rc, err := fetchMetadata(ctx, path)
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
