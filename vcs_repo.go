package gps

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/vcs"
)

type ctxRepo interface {
	vcs.Repo
	get(context.Context) error
	update(context.Context) error
	//updateVersion(context.Context) error
	//ping(context.Context) (bool, error)
}

// original implementation of these methods come from
// https://github.com/Masterminds/vcs

type gitRepo struct {
	*vcs.GitRepo
}

func newVcsRemoteErrorOr(msg string, err error, out string) error {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return err
	}
	return vcs.NewRemoteError(msg, err, out)
}

func newVcsLocalErrorOr(msg string, err error, out string) error {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return err
	}
	return vcs.NewLocalError(msg, err, out)
}

func (r *gitRepo) get(ctx context.Context) error {
	out, err := runFromCwd(ctx, "git", "clone", "--recursive", r.Remote(), r.LocalPath())

	// There are some windows cases where Git cannot create the parent
	// directory, of the location where it's trying to create the repo. Catch
	// that error and try to handle it.
	if err != nil && r.isUnableToCreateDir(err) {
		basePath := filepath.Dir(filepath.FromSlash(r.LocalPath()))
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			err = os.MkdirAll(basePath, 0755)
			if err != nil {
				return newVcsLocalErrorOr("unable to create directory", err, "")
			}

			out, err = runFromCwd(ctx, "git", "clone", r.Remote(), r.LocalPath())
			if err != nil {
				return newVcsRemoteErrorOr("unable to get repository", err, string(out))
			}
			return err
		}
	} else if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *gitRepo) update(ctx context.Context) error {
	// Perform a fetch to make sure everything is up to date.
	//out, err := runFromRepoDir(ctx, r, "git", "fetch", "--tags", "--prune", r.RemoteLocation)
	out, err := runFromRepoDir(ctx, r, "git", "fetch", "--tags", r.RemoteLocation)
	if err != nil {
		return newVcsRemoteErrorOr("unable to update repository", err, string(out))
	}
	return nil
}

func (r *gitRepo) updateVersion(ctx context.Context, v string) error {
	out, err := runFromRepoDir(ctx, r, "git", "checkout", v)
	if err != nil {
		return newVcsLocalErrorOr("Unable to update checked out version", err, string(out))
	}

	return r.defendAgainstSubmodules(ctx)
}

// defendAgainstSubmodules tries to keep repo state sane in the event of
// submodules. Or nested submodules. What a great idea, submodules.
func (r *gitRepo) defendAgainstSubmodules(ctx context.Context) error {
	// First, update them to whatever they should be, if there should happen to be any.
	out, err := runFromRepoDir(ctx, r, "git", "submodule", "update", "--init", "--recursive")
	if err != nil {
		return newVcsLocalErrorOr("unexpected error while defensively updating submodules", err, string(out))
	}

	// Now, do a special extra-aggressive clean in case changing versions caused
	// one or more submodules to go away.
	out, err = runFromRepoDir(ctx, r, "git", "clean", "-x", "-d", "-f", "-f")
	if err != nil {
		return newVcsLocalErrorOr("unexpected error while defensively cleaning up after possible derelict submodule directories", err, string(out))
	}

	// Then, repeat just in case there are any nested submodules that went away.
	out, err = runFromRepoDir(ctx, r, "git", "submodule", "foreach", "--recursive", "git", "clean", "-x", "-d", "-f", "-f")
	if err != nil {
		return newVcsLocalErrorOr("unexpected error while defensively cleaning up after possible derelict nested submodule directories", err, string(out))
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

type bzrRepo struct {
	*vcs.BzrRepo
}

func (r *bzrRepo) get(ctx context.Context) error {
	basePath := filepath.Dir(filepath.FromSlash(r.LocalPath()))
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		err = os.MkdirAll(basePath, 0755)
		if err != nil {
			return newVcsLocalErrorOr("unable to create directory", err, "")
		}
	}

	out, err := runFromCwd(ctx, "bzr", "branch", r.Remote(), r.LocalPath())
	if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *bzrRepo) update(ctx context.Context) error {
	out, err := runFromRepoDir(ctx, r, "bzr", "pull")
	if err != nil {
		return newVcsRemoteErrorOr("unable to update repository", err, string(out))
	}

	out, err = runFromRepoDir(ctx, r, "bzr", "update")
	if err != nil {
		return newVcsRemoteErrorOr("unable to update repository", err, string(out))
	}

	return nil
}

type hgRepo struct {
	*vcs.HgRepo
}

func (r *hgRepo) get(ctx context.Context) error {
	out, err := runFromCwd(ctx, "hg", "clone", r.Remote(), r.LocalPath())
	if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *hgRepo) update(ctx context.Context) error {
	return r.updateVersion(ctx, "")
}

func (r *hgRepo) updateVersion(ctx context.Context, version string) error {
	out, err := runFromRepoDir(ctx, r, "hg", "pull")
	if err != nil {
		return newVcsRemoteErrorOr("unable to update checked out version", err, string(out))
	}

	if len(strings.TrimSpace(version)) > 0 {
		out, err = runFromRepoDir(ctx, r, "hg", "update", version)
	} else {
		out, err = runFromRepoDir(ctx, r, "hg", "update")
	}

	if err != nil {
		return newVcsRemoteErrorOr("unable to update checked out version", err, string(out))
	}

	return nil
}

type svnRepo struct {
	*vcs.SvnRepo
}

func (r *svnRepo) get(ctx context.Context) error {
	remote := r.Remote()
	if strings.HasPrefix(remote, "/") {
		remote = "file://" + remote
	} else if runtime.GOOS == "windows" && filepath.VolumeName(remote) != "" {
		remote = "file:///" + remote
	}

	out, err := runFromCwd(ctx, "svn", "checkout", remote, r.LocalPath())
	if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *svnRepo) update(ctx context.Context) error {
	out, err := runFromRepoDir(ctx, r, "svn", "update")
	if err != nil {
		return newVcsRemoteErrorOr("unable to update repository", err, string(out))
	}

	return err
}

func (r *svnRepo) updateVersion(ctx context.Context, version string) error {
	out, err := runFromRepoDir(ctx, r, "svn", "update", "-r", version)
	if err != nil {
		return newVcsRemoteErrorOr("unable to update checked out version", err, string(out))
	}

	return nil
}

func (r *svnRepo) CommitInfo(id string) (*vcs.CommitInfo, error) {
	ctx := context.TODO()
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

		out, err := runFromRepoDir(ctx, r, "svn", "info", "-r", id, "--xml")
		if err != nil {
			return nil, newVcsLocalErrorOr("unable to retrieve commit information", err, string(out))
		}

		infos := new(info)
		err = xml.Unmarshal(out, &infos)
		if err != nil {
			return nil, newVcsLocalErrorOr("unable to retrieve commit information", err, string(out))
		}

		id = infos.Commit.Revision
		if id == "" {
			return nil, vcs.ErrRevisionUnavailable
		}
	}

	out, err := runFromRepoDir(ctx, r, "svn", "log", "-r", id, "--xml")
	if err != nil {
		return nil, newVcsRemoteErrorOr("unable to retrieve commit information", err, string(out))
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
		return nil, newVcsLocalErrorOr("unable to retrieve commit information", err, string(out))
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
			return nil, newVcsLocalErrorOr("unable to retrieve commit information", err, string(out))
		}
	}

	return ci, nil
}
