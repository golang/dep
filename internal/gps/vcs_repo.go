// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
	fetch(context.Context) error
	updateVersion(context.Context, string) error
	//ping(context.Context) (bool, error)
}

func newCtxRepo(s vcs.Type, ustr, path string) (r ctxRepo, err error) {
	r, err = getVCSRepo(s, ustr, path)
	if err != nil {
		// if vcs could not initialize the repo due to a local error
		// then the local repo is in an incorrect state. Remove and
		// treat it as a new not-yet-cloned repo.

		// TODO(marwan-at-work): warn/give progress of the above comment.
		os.RemoveAll(path)
		r, err = getVCSRepo(s, ustr, path)
	}

	return
}

func getVCSRepo(s vcs.Type, ustr, path string) (r ctxRepo, err error) {
	switch s {
	case vcs.Git:
		var repo *vcs.GitRepo
		repo, err = vcs.NewGitRepo(ustr, path)
		r = &gitRepo{repo}
	case vcs.Bzr:
		var repo *vcs.BzrRepo
		repo, err = vcs.NewBzrRepo(ustr, path)
		r = &bzrRepo{repo}
	case vcs.Hg:
		var repo *vcs.HgRepo
		repo, err = vcs.NewHgRepo(ustr, path)
		r = &hgRepo{repo}
	}

	return
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
	out, err := runFromCwd(ctx, expensiveCmdTimeout, "git", "clone", "--recursive", "-v", "--progress", r.Remote(), r.LocalPath())
	if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *gitRepo) fetch(ctx context.Context) error {
	// Perform a fetch to make sure everything is up to date.
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "git", "fetch", "--tags", "--prune", r.RemoteLocation)
	if err != nil {
		return newVcsRemoteErrorOr("unable to update repository", err, string(out))
	}
	return nil
}

func (r *gitRepo) updateVersion(ctx context.Context, v string) error {
	out, err := runFromRepoDir(ctx, r, defaultCmdTimeout, "git", "checkout", v)
	if err != nil {
		return newVcsLocalErrorOr("Unable to update checked out version", err, string(out))
	}

	return r.defendAgainstSubmodules(ctx)
}

// defendAgainstSubmodules tries to keep repo state sane in the event of
// submodules. Or nested submodules. What a great idea, submodules.
func (r *gitRepo) defendAgainstSubmodules(ctx context.Context) error {
	// First, update them to whatever they should be, if there should happen to be any.
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "git", "submodule", "update", "--init", "--recursive")
	if err != nil {
		return newVcsLocalErrorOr("unexpected error while defensively updating submodules", err, string(out))
	}

	// Now, do a special extra-aggressive clean in case changing versions caused
	// one or more submodules to go away.
	out, err = runFromRepoDir(ctx, r, defaultCmdTimeout, "git", "clean", "-x", "-d", "-f", "-f")
	if err != nil {
		return newVcsLocalErrorOr("unexpected error while defensively cleaning up after possible derelict submodule directories", err, string(out))
	}

	// Then, repeat just in case there are any nested submodules that went away.
	out, err = runFromRepoDir(ctx, r, defaultCmdTimeout, "git", "submodule", "foreach", "--recursive", "git", "clean", "-x", "-d", "-f", "-f")
	if err != nil {
		return newVcsLocalErrorOr("unexpected error while defensively cleaning up after possible derelict nested submodule directories", err, string(out))
	}

	return nil
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

	out, err := runFromCwd(ctx, expensiveCmdTimeout, "bzr", "branch", r.Remote(), r.LocalPath())
	if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *bzrRepo) fetch(ctx context.Context) error {
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "bzr", "pull")
	if err != nil {
		return newVcsRemoteErrorOr("unable to update repository", err, string(out))
	}
	return nil
}

func (r *bzrRepo) updateVersion(ctx context.Context, version string) error {
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "bzr", "update", "-r", version)
	if err != nil {
		return newVcsLocalErrorOr("unable to update checked out version", err, string(out))
	}
	return nil
}

type hgRepo struct {
	*vcs.HgRepo
}

func (r *hgRepo) get(ctx context.Context) error {
	out, err := runFromCwd(ctx, expensiveCmdTimeout, "hg", "clone", r.Remote(), r.LocalPath())
	if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *hgRepo) fetch(ctx context.Context) error {
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "hg", "pull")
	if err != nil {
		return newVcsRemoteErrorOr("unable to fetch latest changes", err, string(out))
	}
	return nil
}

func (r *hgRepo) updateVersion(ctx context.Context, version string) error {
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "hg", "update", version)
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

	out, err := runFromCwd(ctx, expensiveCmdTimeout, "svn", "checkout", remote, r.LocalPath())
	if err != nil {
		return newVcsRemoteErrorOr("unable to get repository", err, string(out))
	}

	return nil
}

func (r *svnRepo) update(ctx context.Context) error {
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "svn", "update")
	if err != nil {
		return newVcsRemoteErrorOr("unable to update repository", err, string(out))
	}

	return err
}

func (r *svnRepo) updateVersion(ctx context.Context, version string) error {
	out, err := runFromRepoDir(ctx, r, expensiveCmdTimeout, "svn", "update", "-r", version)
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

		out, err := runFromRepoDir(ctx, r, defaultCmdTimeout, "svn", "info", "-r", id, "--xml")
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

	out, err := runFromRepoDir(ctx, r, defaultCmdTimeout, "svn", "log", "-r", id, "--xml")
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
