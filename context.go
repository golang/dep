// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/vcs"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

// Ctx defines the supporting context of dep.
type Ctx struct {
	WorkingDir string      // Where to execute.
	GOPATH     string      // Selected Go path, containing WorkingDir.
	GOPATHs    []string    // Other Go paths.
	Out, Err   *log.Logger // Required loggers.
	Verbose    bool        // Enables more verbose logging.
}

func NewContext(wd string, gopaths []string, out, err *log.Logger, verbose bool) *Ctx {
	ctx := &Ctx{
		WorkingDir: wd,
		Out:        out,
		Err:        err,
		Verbose:    verbose,
	}
	for _, gp := range gopaths {
		ctx.GOPATHs = append(ctx.GOPATHs, filepath.ToSlash(gp))
	}

	return ctx
}

func (c *Ctx) SourceManager() (*gps.SourceMgr, error) {
	return gps.NewSourceManager(filepath.Join(c.GOPATH, "pkg", "dep"))
}

// LoadProject starts from the current working directory and searches up the
// directory tree for a project root.  The search stops when a file with the name
// ManifestName (Gopkg.toml, by default) is located.
//
// The Project contains the parsed manifest as well as a parsed lock file, if
// present.  The import path is calculated as the remaining path segment
// below Ctx.GOPATH/src.
func (c *Ctx) LoadProject() (*Project, error) {
	var err error
	p := new(Project)

	p.AbsRoot, err = findProjectRoot(c.WorkingDir)
	if err != nil {
		return nil, err
	}

	// The path may lie within a symlinked directory, resolve the path
	// before moving forward
	p.AbsRoot, c.GOPATH, err = c.ResolveProjectRootAndGOPATH(p.AbsRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve project root")
	} else if c.GOPATH == "" {
		return nil, errors.New("project not within a GOPATH")
	}

	ip, err := c.SplitAbsoluteProjectRoot(p.AbsRoot)
	if err != nil {
		return nil, errors.Wrap(err, "split absolute project root")
	}
	p.ImportRoot = gps.ProjectRoot(ip)

	mp := filepath.Join(p.AbsRoot, ManifestName)
	mf, err := os.Open(mp)
	if err != nil {
		if os.IsNotExist(err) {
			// TODO: list possible solutions? (dep init, cd $project)
			return nil, errors.Errorf("no %v found in project root %v", ManifestName, p.AbsRoot)
		}
		// Unable to read the manifest file
		return nil, err
	}
	defer mf.Close()

	var warns []error
	p.Manifest, warns, err = readManifest(mf)
	for _, warn := range warns {
		c.Err.Printf("dep: WARNING: %v\n", warn)
	}
	if err != nil {
		return nil, errors.Errorf("error while parsing %s: %s", mp, err)
	}

	lp := filepath.Join(p.AbsRoot, LockName)
	lf, err := os.Open(lp)
	if err != nil {
		if os.IsNotExist(err) {
			// It's fine for the lock not to exist
			return p, nil
		}
		// But if a lock does exist and we can't open it, that's a problem
		return nil, errors.Errorf("could not open %s: %s", lp, err)
	}
	defer lf.Close()

	p.Lock, err = readLock(lf)
	if err != nil {
		return nil, errors.Errorf("error while parsing %s: %s", lp, err)
	}

	return p, nil
}

// ResolveProjectRootAndGOPATH evaluates the project root and the containing GOPATH
// by doing the following:
//
// If path isn't a symlink and is within a GOPATH, path and its GOPATH are returned.
//
// If path is a symlink not within any GOPATH and resolves to a directory within a
// GOPATH, the resolved path and its GOPATH are returned.
//
// ResolveProjectRootAndGOPATH will return an error in the following cases:
//
// If path is not a symlink and it's not within any GOPATH.
// If both path and the directory it resolves to are not within any GOPATH.
// If path is a symlink within a GOPATH, an error is returned.
// If both path and the directory it resolves to are within the same GOPATH.
// If path and the directory it resolves to are each within a different GOPATH.
func (c *Ctx) ResolveProjectRootAndGOPATH(path string) (string, string, error) {
	pgp, pgperr := c.detectGOPATH(path)

	if sym, err := fs.IsSymlink(path); err != nil {
		return "", "", errors.Wrap(err, "IsSymlink")
	} else if !sym {
		// If path is not a symlink and detectGOPATH() failed, then we assume that path is not
		// within a known GOPATH.
		if pgperr != nil {
			return "", "", errors.Errorf("project root %v not within a GOPATH", path)
		}
		return path, pgp, nil
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "", errors.Wrap(err, "resolveProjectRoot")
	}

	rgp, rgperr := c.detectGOPATH(resolved)
	if pgperr != nil && rgperr != nil {
		return "", "", errors.Errorf("path %s resolved to %s, both are not within any GOPATH", path, resolved)
	}

	// If pgp equals rgp, then both are within the same GOPATH.
	if pgp == rgp {
		return "", "", errors.Errorf("path %s resolved to %s, both are in the same GOPATH %s", path, resolved, pgp)
	}

	// path and resolved are within different GOPATHs
	if pgp != "" && rgp != "" && pgp == rgp {
		return "", "", errors.Errorf("path %s resolved to %s, each is in a different GOPATH", path, resolved)
	}

	// Otherwise, either the symlink or the resolved path is within a GOPATH.
	if pgp == "" {
		return resolved, rgp, nil
	} else {
		return path, pgp, nil
	}
}

// detectGOPATH detects the GOPATH for a given path from ctx.GOPATHs.
func (c *Ctx) detectGOPATH(path string) (string, error) {
	for _, gp := range c.GOPATHs {
		if fs.HasFilepathPrefix(filepath.FromSlash(path), gp) {
			return gp, nil
		}
	}

	return "", errors.Errorf("unable to detect GOPATH for %s", path)
}

// SplitAbsoluteProjectRoot takes an absolute path and compares it against declared
// GOPATH(s) to determine what portion of the input path should be treated as an
// import path - as a project root.
//
// The second returned string indicates which GOPATH value was used.
func (c *Ctx) SplitAbsoluteProjectRoot(path string) (string, error) {
	srcprefix := filepath.Join(c.GOPATH, "src") + string(filepath.Separator)
	if fs.HasFilepathPrefix(path, srcprefix) {
		if len(path) <= len(srcprefix) {
			return "", errors.New("dep does not currently support using $GOPATH/src as the project root.")
		}

		// filepath.ToSlash because we're dealing with an import path now,
		// not an fs path
		return filepath.ToSlash(path[len(srcprefix):]), nil
	}

	return "", errors.Errorf("%s not in any $GOPATH", path)
}

// absoluteProjectRoot determines the absolute path to the project root
// including the $GOPATH. This will not work with stdlib packages and the
// package directory needs to exist.
func (c *Ctx) absoluteProjectRoot(path string) (string, error) {
	posspath := filepath.Join(c.GOPATH, "src", path)
	dirOK, err := fs.IsDir(posspath)
	if err != nil {
		return "", errors.Wrapf(err, "checking if %s is a directory", posspath)
	}
	if !dirOK {
		return "", errors.Errorf("%s does not exist", posspath)
	}
	return posspath, nil
}

func (c *Ctx) VersionInWorkspace(root gps.ProjectRoot) (gps.Version, error) {
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

	// First look through tags.
	tags, err := repo.Tags()
	if err != nil {
		return nil, errors.Wrapf(err, "getting repo tags for root: %s", pr)
	}
	// Try to match the current version to a tag.
	if contains(tags, ver) {
		// Assume semver if it starts with a v.
		if strings.HasPrefix(ver, "v") {
			return gps.NewVersion(ver).Is(gps.Revision(rev)), nil
		}

		return nil, errors.Errorf("version for root %s does not start with a v: %q", pr, ver)
	}

	// Look for the current branch.
	branches, err := repo.Branches()
	if err != nil {
		return nil, errors.Wrapf(err, "getting repo branch for root: %s")
	}
	// Try to match the current version to a branch.
	if contains(branches, ver) {
		return gps.NewBranch(ver).Is(gps.Revision(rev)), nil
	}

	return gps.Revision(rev), nil
}

// contains checks if a array of strings contains a value
func contains(a []string, b string) bool {
	for _, v := range a {
		if b == v {
			return true
		}
	}
	return false
}
