// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Masterminds/vcs"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

/*
Ctx defines the supporting context of the tool.
A properly initialized Ctx has a GOPATH containing WorkingDir, and non-nil Loggers.

	ctx := &dep.Ctx{
		WorkingDir: gopath + "/src/project/root",
		GOPATH: gopath,
		Out: log.New(os.Stdout, "", 0),
		Err: log.New(os.Stderr, "", 0),
	}

SetPaths assists with setting consistent path fields.

	ctx := &dep.Ctx{
		Out: log.New(os.Stdout, "", 0),
		Err: log.New(os.Stderr, "", 0),
	}
	err := ctx.SetPaths(projectRootPath, filepath.SplitList(os.Getenv("GOPATH"))
	if err != nil {
		// projectRootPath not in any GOPATH
	}

*/
type Ctx struct {
	WorkingDir string      // Where to execute.
	GOPATH     string      // Selected Go path, containing WorkingDir.
	GOPATHS    []string    // Other Go paths.
	Out, Err   *log.Logger // Required loggers.
	Verbose    bool        // Enables more verbose logging.
}

/*
SetPaths sets the WorkingDir, GOPATH, and GOPATHS fields.
It selects the GOPATH containing WorkingDir, or returns an error if none is found.

	err := ctx.SetPaths(projectRootPath, filepath.SplitList(os.Getenv("GOPATH"))
	if err != nil {
		// project root not in any GOPATH
	}

The default GOPATH is checked when none are provided.

	err := ctx.SetPaths(projectRootPath)
	if err != nil {
		// project root not in default GOPATH, or none available
	}

*/
func (c *Ctx) SetPaths(workingDir string, gopaths ...string) error {
	c.WorkingDir = workingDir
	if len(gopaths) == 0 {
		d := defaultGOPATH()
		if d == "" {
			return errors.New("no default GOPATH available")
		}
		gopaths = []string{d}
	}
	wd := filepath.FromSlash(workingDir)
	for _, gp := range gopaths {
		gp = filepath.FromSlash(gp)

		ok, err := hasFilepathPrefix(wd, gp)
		if err != nil {
			return errors.Wrap(err, "failed to check the path")
		}
		if ok {
			c.GOPATH = gp
		}

		c.GOPATHS = append(c.GOPATHS, gp)
	}

	if c.GOPATH == "" {
		return fmt.Errorf("%q not in any GOPATH", wd)
	}

	return nil
}

// defaultGOPATH gets the default GOPATH that was added in 1.8
// copied from go/build/build.go
func defaultGOPATH() string {
	env := "HOME"
	if runtime.GOOS == "windows" {
		env = "USERPROFILE"
	} else if runtime.GOOS == "plan9" {
		env = "home"
	}
	if home := os.Getenv(env); home != "" {
		def := filepath.Join(home, "go")
		if def == runtime.GOROOT() {
			// Don't set the default GOPATH to GOROOT,
			// as that will trigger warnings from the go tool.
			return ""
		}
		return def
	}
	return ""
}

// Similar to fs.HasFilepathPrefix() but aware of symlinks.
func hasFilepathPrefix(path, prefix string) (bool, error) {
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, errors.Wrap(err, "failed to resolve the symlink")
	}
	pre, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		return false, errors.Wrap(err, "failed to resolve the symlink")
	}
	return fs.HasFilepathPrefix(p, pre), nil
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
	p.AbsRoot, err = c.resolveProjectRoot(p.AbsRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve project root")
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

// resolveProjectRoot evaluates the root directory and does the following:
//
// If the passed path is a symlink outside GOPATH to a directory within a
// GOPATH, the resolved full real path is returned.
//
// If the passed path is a symlink within a GOPATH, we return an error.
//
// If the passed path isn't a symlink at all, we just pass through.
func (c *Ctx) resolveProjectRoot(path string) (string, error) {
	// Determine if this path is a Symlink
	l, err := os.Lstat(path)
	if err != nil {
		return "", errors.Wrap(err, "resolveProjectRoot")
	}

	// Pass through if not
	if l.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}

	// Resolve path
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.Wrap(err, "resolveProjectRoot")
	}

	// Determine if the symlink is within any of the GOPATHs, in which case we're not
	// sure how to resolve it.
	for _, gp := range c.GOPATHS {
		if fs.HasFilepathPrefix(path, gp) {
			return "", errors.Errorf("'%s' is linked to another path within a GOPATH (%s)", path, gp)
		}
	}

	return resolved, nil
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
			return gps.NewVersion(ver).Pair(gps.Revision(rev)), nil
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
		return gps.NewBranch(ver).Pair(gps.Revision(rev)), nil
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
