// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/vcs"
	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

// ctx defines the supporting context of the tool.
type ctx struct {
	GOPATH string // Go path
}

// newContext creates a struct with the project's GOPATH. It assumes
// that of your "GOPATH"'s we want the one we are currently in.
func newContext() (*ctx, error) {
	// this way we get the default GOPATH that was added in 1.8
	buildContext := build.Default
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "getting work directory")
	}
	for _, gp := range filepath.SplitList(buildContext.GOPATH) {
		if strings.HasPrefix(wd, gp) {
			return &ctx{GOPATH: gp}, nil
		}
	}

	return nil, errors.New("project not in a GOPATH")
}

func (c *ctx) sourceManager() (*gps.SourceMgr, error) {
	return gps.NewSourceManager(analyzer{}, filepath.Join(c.GOPATH, "pkg", "depcache"))
}

// loadProject searches for a project root from the provided path, then loads
// the manifest and lock (if any) it finds there.
//
// If the provided path is empty, it will search from the path indicated by
// os.Getwd().
func (c *ctx) loadProject(path string) (*project, error) {
	var err error
	p := new(project)

	switch path {
	case "":
		p.absroot, err = findProjectRootFromWD()
	default:
		p.absroot, err = findProjectRoot(path)
	}

	if err != nil {
		return p, err
	}

	ip, err := c.splitAbsoluteProjectRoot(p.absroot)
	if err != nil {
		return nil, errors.Wrap(err, "split absolute project root")
	}
	p.importroot = gps.ProjectRoot(ip)

	mp := filepath.Join(p.absroot, manifestName)
	mf, err := os.Open(mp)
	if err != nil {
		if os.IsNotExist(err) {
			// TODO: list possible solutions? (dep init, cd $project)
			return nil, fmt.Errorf("no %v found in project root %v", manifestName, p.absroot)
		}
		// Unable to read the manifest file
		return nil, err
	}
	defer mf.Close()

	p.m, err = readManifest(mf)
	if err != nil {
		return nil, fmt.Errorf("error while parsing %s: %s", mp, err)
	}

	lp := filepath.Join(path, lockName)
	lf, err := os.Open(lp)
	if err != nil {
		if os.IsNotExist(err) {
			// It's fine for the lock not to exist
			return p, nil
		}
		// But if a lock does exist and we can't open it, that's a problem
		return nil, fmt.Errorf("could not open %s: %s", lp, err)
	}

	defer lf.Close()
	p.l, err = readLock(lf)
	if err != nil {
		return nil, fmt.Errorf("error while parsing %s: %s", lp, err)
	}

	return p, nil
}

// splitAbsoluteProjectRoot takes an absolute path and compares it against declared
// GOPATH(s) to determine what portion of the input path should be treated as an
// import path - as a project root.
//
// The second returned string indicates which GOPATH value was used.
func (c *ctx) splitAbsoluteProjectRoot(path string) (string, error) {
	srcprefix := filepath.Join(c.GOPATH, "src") + string(filepath.Separator)
	if strings.HasPrefix(path, srcprefix) {
		// filepath.ToSlash because we're dealing with an import path now,
		// not an fs path
		return filepath.ToSlash(strings.TrimPrefix(path, srcprefix)), nil
	}

	return "", fmt.Errorf("%s not in any $GOPATH", path)
}

// absoluteProjectRoot determines the absolute path to the project root
// including the $GOPATH. This will not work with stdlib packages and the
// package directory needs to exist.
func (c *ctx) absoluteProjectRoot(path string) (string, error) {
	posspath := filepath.Join(c.GOPATH, "src", path)
	dirOK, err := isDir(posspath)
	if err != nil {
		return "", errors.Wrapf(err, "checking if %s is a directory", posspath)
	}

	if !dirOK {
		return "", fmt.Errorf("%s does not exist", posspath)
	}

	return posspath, nil
}

func (c *ctx) versionInWorkspace(root gps.ProjectRoot) (gps.Version, error) {
	pr, err := c.absoluteProjectRoot(string(root))
	if err != nil {
		return nil, errors.Wrapf(err, "determine project root for %s", root)
	}

	repo, err := vcs.NewRepo("", pr)
	if err != nil {
		return nil, errors.Wrapf(err, "creating new repo for root: %s", pr)
	}

	ver, err := repo.Current()
	if err != nil {
		return nil, errors.Wrapf(err, "finding current branch/version for root: %s", pr)
	}

	rev, err := repo.Version()
	if err != nil {
		return nil, errors.Wrapf(err, "getting repo version for root: %s", pr)
	}

	// first look through tags
	tags, err := repo.Tags()
	if err != nil {
		return nil, errors.Wrapf(err, "getting repo tags for root: %s", pr)
	}
	// try to match the current version to a tag
	if contains(tags, ver) {
		// assume semver if it starts with a v
		if strings.HasPrefix(ver, "v") {
			return gps.NewVersion(ver).Is(gps.Revision(rev)), nil
		}

		return nil, fmt.Errorf("version for root %s does not start with a v: %q", pr, ver)
	}

	// look for the current branch
	branches, err := repo.Branches()
	if err != nil {
		return nil, errors.Wrapf(err, "getting repo branch for root: %s")
	}
	// try to match the current version to a branch
	if contains(branches, ver) {
		return gps.NewBranch(ver).Is(gps.Revision(rev)), nil
	}

	return gps.Revision(rev), nil
}
