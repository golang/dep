// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
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

// Ctx defines the supporting context of dep.
//
// A properly initialized Ctx has a GOPATH containing the project root and non-nil Loggers.
//
//	ctx := &dep.Ctx{
//		WorkingDir: GOPATH + "/src/project/root",
//		GOPATH: GOPATH,
//		Out: log.New(os.Stdout, "", 0),
//		Err: log.New(os.Stderr, "", 0),
//	}
//
// Ctx.DetectProjectGOPATH() helps with setting the containing GOPATH.
//
//	ctx.GOPATH, err := Ctx.DetectProjectGOPATH(project)
//	if err != nil {
//		// Could not determine which GOPATH to use for the project.
//	}
//
type Ctx struct {
	WorkingDir string      // Where to execute.
	GOPATH     string      // Selected Go path, containing WorkingDir.
	GOPATHs    []string    // Other Go paths.
	Out, Err   *log.Logger // Required loggers.
	Verbose    bool        // Enables more verbose logging.
}

// SetPaths sets the WorkingDir and GOPATHSs fields.
//
//	ctx := &dep.Ctx{
//		Out: log.New(os.Stdout, "", 0),
//		Err: log.New(os.Stderr, "", 0),
//	}
//
//	err := ctx.SetPaths(workingDir, filepath.SplitList(os.Getenv("GOPATH"))
//	if err != nil {
//		// Empty GOPATH
//	}
//
func (c *Ctx) SetPaths(wd string, GOPATHs ...string) error {
	if wd == "" {
		return errors.New("cannot set Ctx.WorkingDir to an empty path")
	}
	c.WorkingDir = wd

	if len(GOPATHs) == 0 {
		GOPATHs = getGOPATHs(os.Environ())
	}
	for _, gp := range GOPATHs {
		c.GOPATHs = append(c.GOPATHs, filepath.ToSlash(gp))
	}

	return nil
}

// getGOPATH returns the GOPATHs from the passed environment variables.
// If GOPATH is not defined, fallback to defaultGOPATH().
func getGOPATHs(env []string) []string {
	GOPATH := os.Getenv("GOPATH")
	if GOPATH == "" {
		GOPATH = defaultGOPATH()
	}

	return filepath.SplitList(GOPATH)
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
	root, err := findProjectRoot(c.WorkingDir)
	if err != nil {
		return nil, err
	}

	p := new(Project)

	if err = p.SetRoot(root); err != nil {
		return nil, err
	}

	c.GOPATH, err = c.DetectProjectGOPATH(p)
	if err != nil {
		return nil, err
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

// DetectProjectGOPATH attempt to find the GOPATH containing the project.
//
//  If p.AbsRoot is not a symlink and is within a GOPATH, the GOPATH containing p.AbsRoot is returned.
//  If p.AbsRoot is a symlink and is not within any known GOPATH, the GOPATH containing p.ResolvedAbsRoot is returned.
//
// p.AbsRoot is assumed to be a symlink if it is not the same as p.ResolvedAbsRoot.
//
// DetectProjectGOPATH will return an error in the following cases:
//
//  If p.AbsRoot is not a symlink and is not within any known GOPATH.
//  If neither p.AbsRoot nor p.ResolvedAbsRoot are within a known GOPATH.
//  If both p.AbsRoot and p.ResolvedAbsRoot are within the same GOPATH.
//  If p.AbsRoot and p.ResolvedAbsRoot are each within a different GOPATH.
func (c *Ctx) DetectProjectGOPATH(p *Project) (string, error) {
	if p.AbsRoot == "" || p.ResolvedAbsRoot == "" {
		return "", errors.New("project AbsRoot and ResolvedAbsRoot must be set to detect GOPATH")
	}

	pGOPATH, perr := c.detectGOPATH(p.AbsRoot)

	// If p.AbsRoot is a not symlink, attempt to detect GOPATH for p.AbsRoot only.
	if p.AbsRoot == p.ResolvedAbsRoot {
		return pGOPATH, perr
	}

	rGOPATH, rerr := c.detectGOPATH(p.ResolvedAbsRoot)

	// If detectGOPATH() failed for both p.AbsRoot and p.ResolvedAbsRoot, then both are not within any known GOPATHs.
	if perr != nil && rerr != nil {
		return "", errors.Errorf("both %s and %s are not within any known GOPATH", p.AbsRoot, p.ResolvedAbsRoot)
	}

	// If pGOPATH equals rGOPATH, then both are within the same GOPATH.
	if pGOPATH == rGOPATH {
		return "", errors.Errorf("both %s and %s are in the same GOPATH %s", p.AbsRoot, p.ResolvedAbsRoot, pGOPATH)
	}

	if pGOPATH != "" && rGOPATH != "" {
		return "", errors.Errorf("%s and %s are both in different GOPATHs", p.AbsRoot, p.ResolvedAbsRoot)
	}

	// Otherwise, either the p.AbsRoot or p.ResolvedAbsRoot is within a GOPATH.
	if pGOPATH == "" {
		return rGOPATH, nil
	}

	return pGOPATH, nil
}

// detectGOPATH detects the GOPATH for a given path from ctx.GOPATHs.
func (c *Ctx) detectGOPATH(path string) (string, error) {
	for _, gp := range c.GOPATHs {
		if fs.HasFilepathPrefix(filepath.FromSlash(path), gp) {
			return gp, nil
		}
	}
	return "", errors.Errorf("%s is not within a known GOPATH", path)
}

// SplitAbsoluteProjectRoot takes an absolute path and compares it against the detected
// GOPATH to determine what portion of the input path should be treated as an
// import path - as a project root.
func (c *Ctx) SplitAbsoluteProjectRoot(path string) (string, error) {
	if c.GOPATH == "" {
		return "", errors.Errorf("no GOPATH detected in this context")
	}

	srcprefix := filepath.Join(c.GOPATH, "src") + string(filepath.Separator)
	if fs.HasFilepathPrefix(path, srcprefix) {
		if len(path) <= len(srcprefix) {
			return "", errors.New("dep does not currently support using GOPATH/src as the project root")
		}

		// filepath.ToSlash because we're dealing with an import path now,
		// not an fs path
		return filepath.ToSlash(path[len(srcprefix):]), nil
	}

	return "", errors.Errorf("%s not in any GOPATH", path)
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
