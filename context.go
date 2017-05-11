// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Masterminds/vcs"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

// Ctx defines the supporting context of the tool.
type Ctx struct {
	GOPATH     string   // Selected Go path
	GOPATHS    []string // Other Go paths
	WorkingDir string
}

// NewContext creates a struct with the project's GOPATH. It assumes
// that of your "GOPATH"'s we want the one we are currently in.
func NewContext(wd string, env []string) (*Ctx, error) {
	ctx := &Ctx{WorkingDir: wd}

	GOPATH := getEnv(env, "GOPATH")
	if GOPATH == "" {
		GOPATH = defaultGOPATH()
	}
	for _, gp := range filepath.SplitList(GOPATH) {
		gp = filepath.FromSlash(gp)

		if fs.HasFilepathPrefix(filepath.FromSlash(wd), gp) {
			ctx.GOPATH = gp
		}

		ctx.GOPATHS = append(ctx.GOPATHS, gp)
	}

	if ctx.GOPATH == "" {
		return nil, errors.New("project not in a GOPATH")
	}

	return ctx, nil
}

// getEnv returns the last instance of an environment variable.
func getEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		v := env[i]
		kv := strings.SplitN(v, "=", 2)
		if kv[0] == key {
			if len(kv) > 1 {
				return kv[1]
			}
			return ""
		}
	}
	return ""
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

// LoadProject takes a path and searches up the directory tree for
// a project root.  If an absolute path is given, the search begins in that
// directory.  If a relative or empty path is given, the search start is computed
// from the current working directory.  The search stops when a file with the
// name ManifestName (Gopkg.toml, by default) is located.
//
// The Project contains the parsed manifest as well as a parsed lock file, if
// present.  The import path is calculated as the remaining path segment
// below Ctx.GOPATH/src.
func (c *Ctx) LoadProject(path string) (*Project, error) {
	var err error
	p := new(Project)

	if path != "" {
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
	}
	switch path {
	case "":
		p.AbsRoot, err = findProjectRoot(c.WorkingDir)
	default:
		p.AbsRoot, err = findProjectRoot(path)
	}

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

	p.Manifest, err = readManifest(mf)
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
