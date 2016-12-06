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

func newContext() *ctx {
	// this way we get the default GOPATH that was added in 1.8
	buildContext := build.Default
	return &ctx{
		GOPATH: buildContext.GOPATH,
	}
}

func (c *ctx) sourceManager() (*gps.SourceMgr, error) {
	if c.GOPATH == "" {
		return nil, fmt.Errorf("GOPATH is not set")
	}
	// Use the first entry in GOPATH for the depcache
	first := filepath.SplitList(c.GOPATH)[0]

	return gps.NewSourceManager(analyzer{}, filepath.Join(first, "depcache"))
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

	var match bool
	for _, gp := range filepath.SplitList(c.GOPATH) {
		srcprefix := filepath.Join(gp, "src") + string(filepath.Separator)
		if strings.HasPrefix(p.absroot, srcprefix) {
			match = true
			// filepath.ToSlash because we're dealing with an import path now,
			// not an fs path
			p.importroot = gps.ProjectRoot(filepath.ToSlash(strings.TrimPrefix(p.absroot, srcprefix)))
			break
		}
	}
	if !match {
		return nil, fmt.Errorf("could not determine project root - not on GOPATH")
	}

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
// TODO: rename this appropriately
func (c *ctx) splitAbsoluteProjectRoot(path string) (string, string, error) {
	for _, gp := range filepath.SplitList(c.GOPATH) {
		srcprefix := filepath.Join(gp, "src") + string(filepath.Separator)
		if strings.HasPrefix(path, srcprefix) {
			// filepath.ToSlash because we're dealing with an import path now,
			// not an fs path
			return filepath.ToSlash(strings.TrimPrefix(path, srcprefix)), gp, nil
		}
	}
	return "", "", fmt.Errorf("%s not in any $GOPATH", path)
}

// absoluteProjectRoot determines the absolute path to the project root
// including the $GOPATH. This will not work with stdlib packages and the
// package directory needs to exist.
func (c *ctx) absoluteProjectRoot(path string) (string, error) {
	for _, gp := range filepath.SplitList(c.GOPATH) {
		posspath := filepath.Join(gp, "src", path)
		dirOK, err := isDir(posspath)
		if err != nil || !dirOK {
			continue
		}

		return posspath, nil
	}
	return "", fmt.Errorf("%s not in any $GOPATH", path)
}

func (c *ctx) versionInWorkspace(root gps.ProjectRoot) (gps.Version, error) {
	pr, err := c.absoluteProjectRoot(string(root))
	if err != nil {
		return nil, errors.Wrapf(err, "determine project root for %s", root)
	}

	repo, err := vcs.NewRepo("", string(pr))
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
