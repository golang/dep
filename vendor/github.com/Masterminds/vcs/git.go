package vcs

import (
	"bytes"
	"encoding/xml"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// NewGitRepo creates a new instance of GitRepo. The remote and local directories
// need to be passed in.
func NewGitRepo(remote, local string) (*GitRepo, error) {
	ins := depInstalled("git")
	if !ins {
		return nil, NewLocalError("git is not installed", nil, "")
	}
	ltype, err := DetectVcsFromFS(local)

	// Found a VCS other than Git. Need to report an error.
	if err == nil && ltype != Git {
		return nil, ErrWrongVCS
	}

	r := &GitRepo{}
	r.setRemote(remote)
	r.setLocalPath(local)
	r.RemoteLocation = "origin"
	r.Logger = Logger

	// Make sure the local Git repo is configured the same as the remote when
	// A remote value was passed in.
	if err == nil && r.CheckLocal() == true {
		c := exec.Command("git", "config", "--get", "remote.origin.url")
		c.Dir = local
		c.Env = envForDir(c.Dir)
		out, err := c.CombinedOutput()
		if err != nil {
			return nil, NewLocalError("Unable to retrieve local repo information", err, string(out))
		}

		localRemote := strings.TrimSpace(string(out))
		if remote != "" && localRemote != remote {
			return nil, ErrWrongRemote
		}

		// If no remote was passed in but one is configured for the locally
		// checked out Git repo use that one.
		if remote == "" && localRemote != "" {
			r.setRemote(localRemote)
		}
	}

	return r, nil
}

// GitRepo implements the Repo interface for the Git source control.
type GitRepo struct {
	base
	RemoteLocation string
}

// Vcs retrieves the underlying VCS being implemented.
func (s GitRepo) Vcs() Type {
	return Git
}

// Get is used to perform an initial clone of a repository.
func (s *GitRepo) Get() error {
	out, err := s.run("git", "clone", s.Remote(), s.LocalPath())

	// There are some windows cases where Git cannot create the parent directory,
	// if it does not already exist, to the location it's trying to create the
	// repo. Catch that error and try to handle it.
	if err != nil && s.isUnableToCreateDir(err) {

		basePath := filepath.Dir(filepath.FromSlash(s.LocalPath()))
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			err = os.MkdirAll(basePath, 0755)
			if err != nil {
				return NewLocalError("Unable to create directory", err, "")
			}

			out, err = s.run("git", "clone", s.Remote(), s.LocalPath())
			if err != nil {
				return NewRemoteError("Unable to get repository", err, string(out))
			}
			return err
		}

	} else if err != nil {
		return NewRemoteError("Unable to get repository", err, string(out))
	}

	return nil
}

// Init initializes a git repository at local location.
func (s *GitRepo) Init() error {
	out, err := s.run("git", "init", s.LocalPath())

	// There are some windows cases where Git cannot create the parent directory,
	// if it does not already exist, to the location it's trying to create the
	// repo. Catch that error and try to handle it.
	if err != nil && s.isUnableToCreateDir(err) {

		basePath := filepath.Dir(filepath.FromSlash(s.LocalPath()))
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			err = os.MkdirAll(basePath, 0755)
			if err != nil {
				return NewLocalError("Unable to initialize repository", err, "")
			}

			out, err = s.run("git", "init", s.LocalPath())
			if err != nil {
				return NewLocalError("Unable to initialize repository", err, string(out))
			}
			return nil
		}

	} else if err != nil {
		return NewLocalError("Unable to initialize repository", err, string(out))
	}

	return nil
}

// Update performs an Git fetch and pull to an existing checkout.
func (s *GitRepo) Update() error {
	// Perform a fetch to make sure everything is up to date.
	out, err := s.RunFromDir("git", "fetch", s.RemoteLocation)
	if err != nil {
		return NewRemoteError("Unable to update repository", err, string(out))
	}

	// When in a detached head state, such as when an individual commit is checked
	// out do not attempt a pull. It will cause an error.
	detached, err := isDetachedHead(s.LocalPath())
	if err != nil {
		return NewLocalError("Unable to update repository", err, "")
	}

	if detached == true {
		return nil
	}

	out, err = s.RunFromDir("git", "pull")
	if err != nil {
		return NewRemoteError("Unable to update repository", err, string(out))
	}
	return nil
}

// UpdateVersion sets the version of a package currently checked out via Git.
func (s *GitRepo) UpdateVersion(version string) error {
	out, err := s.RunFromDir("git", "checkout", version)
	if err != nil {
		return NewLocalError("Unable to update checked out version", err, string(out))
	}
	return nil
}

// Version retrieves the current version.
func (s *GitRepo) Version() (string, error) {
	out, err := s.RunFromDir("git", "rev-parse", "HEAD")
	if err != nil {
		return "", NewLocalError("Unable to retrieve checked out version", err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}

// Current returns the current version-ish. This means:
// * Branch name if on the tip of the branch
// * Tag if on a tag
// * Otherwise a revision id
func (s *GitRepo) Current() (string, error) {
	out, err := s.RunFromDir("git", "symbolic-ref", "HEAD")
	if err == nil {
		o := bytes.TrimSpace(bytes.TrimPrefix(out, []byte("refs/heads/")))
		return string(o), nil
	}

	v, err := s.Version()
	if err != nil {
		return "", err
	}

	ts, err := s.TagsFromCommit(v)
	if err != nil {
		return "", err
	}

	if len(ts) > 0 {
		return ts[0], nil
	}

	return v, nil
}

// Date retrieves the date on the latest commit.
func (s *GitRepo) Date() (time.Time, error) {
	out, err := s.RunFromDir("git", "log", "-1", "--date=iso", "--pretty=format:%cd")
	if err != nil {
		return time.Time{}, NewLocalError("Unable to retrieve revision date", err, string(out))
	}
	t, err := time.Parse(longForm, string(out))
	if err != nil {
		return time.Time{}, NewLocalError("Unable to retrieve revision date", err, string(out))
	}
	return t, nil
}

// Branches returns a list of available branches on the RemoteLocation
func (s *GitRepo) Branches() ([]string, error) {
	out, err := s.RunFromDir("git", "show-ref")
	if err != nil {
		return []string{}, NewLocalError("Unable to retrieve branches", err, string(out))
	}
	branches := s.referenceList(string(out), `(?m-s)(?:`+s.RemoteLocation+`)/(\S+)$`)
	return branches, nil
}

// Tags returns a list of available tags on the RemoteLocation
func (s *GitRepo) Tags() ([]string, error) {
	out, err := s.RunFromDir("git", "show-ref")
	if err != nil {
		return []string{}, NewLocalError("Unable to retrieve tags", err, string(out))
	}
	tags := s.referenceList(string(out), `(?m-s)(?:tags)/(\S+)$`)
	return tags, nil
}

// CheckLocal verifies the local location is a Git repo.
func (s *GitRepo) CheckLocal() bool {
	if _, err := os.Stat(s.LocalPath() + "/.git"); err == nil {
		return true
	}

	return false
}

// IsReference returns if a string is a reference. A reference can be a
// commit id, branch, or tag.
func (s *GitRepo) IsReference(r string) bool {
	_, err := s.RunFromDir("git", "rev-parse", "--verify", r)
	if err == nil {
		return true
	}

	// Some refs will fail rev-parse. For example, a remote branch that has
	// not been checked out yet. This next step should pickup the other
	// possible references.
	_, err = s.RunFromDir("git", "show-ref", r)
	if err == nil {
		return true
	}

	return false
}

// IsDirty returns if the checkout has been modified from the checked
// out reference.
func (s *GitRepo) IsDirty() bool {
	out, err := s.RunFromDir("git", "diff")
	return err != nil || len(out) != 0
}

// CommitInfo retrieves metadata about a commit.
func (s *GitRepo) CommitInfo(id string) (*CommitInfo, error) {
	fm := `--pretty=format:"<logentry><commit>%H</commit><author>%an &lt;%ae&gt;</author><date>%aD</date><message>%s</message></logentry>"`
	out, err := s.RunFromDir("git", "log", id, fm, "-1")
	if err != nil {
		return nil, ErrRevisionUnavailable
	}

	cis := struct {
		Commit  string `xml:"commit"`
		Author  string `xml:"author"`
		Date    string `xml:"date"`
		Message string `xml:"message"`
	}{}
	err = xml.Unmarshal(out, &cis)
	if err != nil {
		return nil, NewLocalError("Unable to retrieve commit information", err, string(out))
	}

	t, err := time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", cis.Date)
	if err != nil {
		return nil, NewLocalError("Unable to retrieve commit information", err, string(out))
	}

	ci := &CommitInfo{
		Commit:  cis.Commit,
		Author:  cis.Author,
		Date:    t,
		Message: cis.Message,
	}

	return ci, nil
}

// TagsFromCommit retrieves tags from a commit id.
func (s *GitRepo) TagsFromCommit(id string) ([]string, error) {
	// This is imperfect and a better method would be great.

	var re []string

	out, err := s.RunFromDir("git", "show-ref", "-d")
	if err != nil {
		return []string{}, NewLocalError("Unable to retrieve tags", err, string(out))
	}

	lines := strings.Split(string(out), "\n")
	var list []string
	for _, i := range lines {
		if strings.HasPrefix(strings.TrimSpace(i), id) {
			list = append(list, i)
		}
	}
	tags := s.referenceList(strings.Join(list, "\n"), `(?m-s)(?:tags)/(\S+)$`)
	for _, t := range tags {
		// Dereferenced tags have ^{} appended to them.
		re = append(re, strings.TrimSuffix(t, "^{}"))
	}

	return re, nil
}

// Ping returns if remote location is accessible.
func (s *GitRepo) Ping() bool {
	c := exec.Command("git", "ls-remote", s.Remote())

	// If prompted for a username and password, which GitHub does for all things
	// not public, it's considered not available. To make it available the
	// remote needs to be different.
	c.Env = mergeEnvLists([]string{"GIT_TERMINAL_PROMPT=0"}, os.Environ())
	_, err := c.CombinedOutput()
	if err != nil {
		return false
	}

	return true
}

// ExportDir exports the current revision to the passed in directory.
func (s *GitRepo) ExportDir(dir string) error {

	// Without the trailing / there can be problems.
	if !strings.HasSuffix(dir, string(os.PathSeparator)) {
		dir = dir + string(os.PathSeparator)
	}

	out, err := s.RunFromDir("git", "checkout-index", "-f", "-a", "--prefix="+dir)
	s.log(out)
	if err != nil {
		return NewLocalError("Unable to export source", err, string(out))
	}

	return nil
}

// isDetachedHead will detect if git repo is in "detached head" state.
func isDetachedHead(dir string) (bool, error) {
	p := filepath.Join(dir, ".git", "HEAD")
	contents, err := ioutil.ReadFile(p)
	if err != nil {
		return false, err
	}

	contents = bytes.TrimSpace(contents)
	if bytes.HasPrefix(contents, []byte("ref: ")) {
		return false, nil
	}

	return true, nil
}

// isUnableToCreateDir checks for an error in Init() to see if an error
// where the parent directory of the VCS local path doesn't exist. This is
// done in a multi-lingual manner.
func (s *GitRepo) isUnableToCreateDir(err error) bool {
	msg := err.Error()
	if strings.HasPrefix(msg, "could not create work tree dir") ||
		strings.HasPrefix(msg, "不能创建工作区目录") ||
		strings.HasPrefix(msg, "no s'ha pogut crear el directori d'arbre de treball") ||
		strings.HasPrefix(msg, "impossible de créer le répertoire de la copie de travail") ||
		strings.HasPrefix(msg, "kunde inte skapa arbetskatalogen") ||
		(strings.HasPrefix(msg, "Konnte Arbeitsverzeichnis") && strings.Contains(msg, "nicht erstellen")) ||
		(strings.HasPrefix(msg, "작업 디렉터리를") && strings.Contains(msg, "만들 수 없습니다")) {
		return true
	}

	return false
}
