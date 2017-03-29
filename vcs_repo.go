package gps

import (
	"bytes"
	"encoding/xml"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/vcs"
)

// original implementation of these methods come from
// https://github.com/Masterminds/vcs

type gitRepo struct {
	*vcs.GitRepo
}

func (r *gitRepo) Get() error {
	out, err := runFromCwd("git", "clone", "--recursive", r.Remote(), r.LocalPath())

	// There are some windows cases where Git cannot create the parent directory,
	// if it does not already exist, to the location it's trying to create the
	// repo. Catch that error and try to handle it.
	if err != nil && r.isUnableToCreateDir(err) {
		basePath := filepath.Dir(filepath.FromSlash(r.LocalPath()))
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			err = os.MkdirAll(basePath, 0755)
			if err != nil {
				return vcs.NewLocalError("unable to create directory", err, "")
			}

			out, err = runFromCwd("git", "clone", r.Remote(), r.LocalPath())
			if err != nil {
				return vcs.NewRemoteError("unable to get repository", err, string(out))
			}
			return err
		}
	} else if err != nil {
		return vcs.NewRemoteError("unable to get repository", err, string(out))
	}

	return nil
}

func (r *gitRepo) Update() error {
	// Perform a fetch to make sure everything is up to date.
	//out, err := runFromRepoDir(r, "git", "fetch", "--tags", "--prune", r.RemoteLocation)
	out, err := runFromRepoDir(r, "git", "fetch", "--tags", r.RemoteLocation)
	if err != nil {
		return vcs.NewRemoteError("unable to update repository", err, string(out))
	}

	// When in a detached head state, such as when an individual commit is checked
	// out do not attempt a pull. It will cause an error.
	detached, err := r.isDetachedHead()
	if err != nil {
		return vcs.NewLocalError("unable to update repository", err, "")
	}

	if detached {
		return nil
	}

	out, err = runFromRepoDir(r, "git", "pull")
	if err != nil {
		return vcs.NewRemoteError("unable to update repository", err, string(out))
	}

	return r.defendAgainstSubmodules()
}

// defendAgainstSubmodules tries to keep repo state sane in the event of
// submodules. Or nested submodules. What a great idea, submodules.
func (r *gitRepo) defendAgainstSubmodules() error {
	// First, update them to whatever they should be, if there should happen to be any.
	out, err := runFromRepoDir(r, "git", "submodule", "update", "--init", "--recursive")
	if err != nil {
		return vcs.NewLocalError("unexpected error while defensively updating submodules", err, string(out))
	}

	// Now, do a special extra-aggressive clean in case changing versions caused
	// one or more submodules to go away.
	out, err = runFromRepoDir(r, "git", "clean", "-x", "-d", "-f", "-f")
	if err != nil {
		return vcs.NewLocalError("unexpected error while defensively cleaning up after possible derelict submodule directories", err, string(out))
	}

	// Then, repeat just in case there are any nested submodules that went away.
	out, err = runFromRepoDir(r, "git", "submodule", "foreach", "--recursive", "git", "clean", "-x", "-d", "-f", "-f")
	if err != nil {
		return vcs.NewLocalError("unexpected error while defensively cleaning up after possible derelict nested submodule directories", err, string(out))
	}

	return nil
}

// isUnableToCreateDir checks for an error in the command to see if an error
// where the parent directory of the VCS local path doesn't exist. This is
// done in a multi-lingual manner.
func (r *gitRepo) isUnableToCreateDir(err error) bool {
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

// isDetachedHead will detect if git repo is in "detached head" state.
func (r *gitRepo) isDetachedHead() (bool, error) {
	p := filepath.Join(r.LocalPath(), ".git", "HEAD")
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

type bzrRepo struct {
	*vcs.BzrRepo
}

func (r *bzrRepo) Get() error {
	basePath := filepath.Dir(filepath.FromSlash(r.LocalPath()))
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		err = os.MkdirAll(basePath, 0755)
		if err != nil {
			return vcs.NewLocalError("unable to create directory", err, "")
		}
	}

	out, err := runFromCwd("bzr", "branch", r.Remote(), r.LocalPath())
	if err != nil {
		return vcs.NewRemoteError("unable to get repository", err, string(out))
	}

	return nil
}

func (r *bzrRepo) Update() error {
	out, err := runFromRepoDir(r, "bzr", "pull")
	if err != nil {
		return vcs.NewRemoteError("unable to update repository", err, string(out))
	}

	out, err = runFromRepoDir(r, "bzr", "update")
	if err != nil {
		return vcs.NewRemoteError("unable to update repository", err, string(out))
	}

	return nil
}

type hgRepo struct {
	*vcs.HgRepo
}

func (r *hgRepo) Get() error {
	out, err := runFromCwd("hg", "clone", r.Remote(), r.LocalPath())
	if err != nil {
		return vcs.NewRemoteError("unable to get repository", err, string(out))
	}

	return nil
}

func (r *hgRepo) Update() error {
	return r.UpdateVersion(``)
}

func (r *hgRepo) UpdateVersion(version string) error {
	out, err := runFromRepoDir(r, "hg", "pull")
	if err != nil {
		return vcs.NewRemoteError("unable to update checked out version", err, string(out))
	}

	if len(strings.TrimSpace(version)) > 0 {
		out, err = runFromRepoDir(r, "hg", "update", version)
	} else {
		out, err = runFromRepoDir(r, "hg", "update")
	}

	if err != nil {
		return vcs.NewRemoteError("unable to update checked out version", err, string(out))
	}

	return nil
}

type svnRepo struct {
	*vcs.SvnRepo
}

func (r *svnRepo) Get() error {
	remote := r.Remote()
	if strings.HasPrefix(remote, "/") {
		remote = "file://" + remote
	} else if runtime.GOOS == "windows" && filepath.VolumeName(remote) != "" {
		remote = "file:///" + remote
	}

	out, err := runFromCwd("svn", "checkout", remote, r.LocalPath())
	if err != nil {
		return vcs.NewRemoteError("unable to get repository", err, string(out))
	}

	return nil
}

func (r *svnRepo) Update() error {
	out, err := runFromRepoDir(r, "svn", "update")
	if err != nil {
		return vcs.NewRemoteError("unable to update repository", err, string(out))
	}

	return err
}

func (r *svnRepo) UpdateVersion(version string) error {
	out, err := runFromRepoDir(r, "svn", "update", "-r", version)
	if err != nil {
		return vcs.NewRemoteError("unable to update checked out version", err, string(out))
	}

	return nil
}

func (r *svnRepo) CommitInfo(id string) (*vcs.CommitInfo, error) {
	// There are cases where Svn log doesn't return anything for HEAD or BASE.
	// svn info does provide details for these but does not have elements like
	// the commit message.
	if id == "HEAD" || id == "BASE" {
		type commit struct {
			Revision string `xml:"revision,attr"`
		}

		type info struct {
			Commit commit `xml:"entry>commit"`
		}

		out, err := runFromRepoDir(r, "svn", "info", "-r", id, "--xml")
		if err != nil {
			return nil, vcs.NewLocalError("unable to retrieve commit information", err, string(out))
		}

		infos := new(info)
		err = xml.Unmarshal(out, &infos)
		if err != nil {
			return nil, vcs.NewLocalError("unable to retrieve commit information", err, string(out))
		}

		id = infos.Commit.Revision
		if id == "" {
			return nil, vcs.ErrRevisionUnavailable
		}
	}

	out, err := runFromRepoDir(r, "svn", "log", "-r", id, "--xml")
	if err != nil {
		return nil, vcs.NewRemoteError("unable to retrieve commit information", err, string(out))
	}

	type logentry struct {
		Author string `xml:"author"`
		Date   string `xml:"date"`
		Msg    string `xml:"msg"`
	}

	type log struct {
		XMLName xml.Name   `xml:"log"`
		Logs    []logentry `xml:"logentry"`
	}

	logs := new(log)
	err = xml.Unmarshal(out, &logs)
	if err != nil {
		return nil, vcs.NewLocalError("unable to retrieve commit information", err, string(out))
	}

	if len(logs.Logs) == 0 {
		return nil, vcs.ErrRevisionUnavailable
	}

	ci := &vcs.CommitInfo{
		Commit:  id,
		Author:  logs.Logs[0].Author,
		Message: logs.Logs[0].Msg,
	}

	if len(logs.Logs[0].Date) > 0 {
		ci.Date, err = time.Parse(time.RFC3339Nano, logs.Logs[0].Date)
		if err != nil {
			return nil, vcs.NewLocalError("unable to retrieve commit information", err, string(out))
		}
	}

	return ci, nil
}
