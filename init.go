// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

var initCmd = &command{
	fn:   runInit,
	name: "init",
	short: `
	Write Manifest file in the root of the project directory.
	`,
	long: `
	Populates Manifest file with current deps of this project.
	The specified version of each dependent repository is the version
	available in the user's workspaces (as specified by GOPATH).
	If the dependency is not present in any workspaces it is not be
	included in the Manifest.
	Writes Lock file(?)
	Creates vendor/ directory(?)

    Notes from DOC:
    Reads existing dependency information written by other tools.
    Noting any information that is lost (unsupported features, etc).
    This functionality will be removed after a transition period (1 year?).
    Write Manifest file in the root of the project directory.
    * Populates Manifest file with current deps of this project.
    The specified version of each dependent repository is the version available in the user's workspaces (including vendor/ directories, if present).
    If the dependency is not present in any workspaces it will not be included in the Manifest. A warning will be issued for these dependencies.
    Creates vendor/ directory (if it does not exist)
    Copies the project’s dependencies from the workspace to the vendor/ directory (if they’re not already there).
    Writes a Lockfile in the root of the project directory.
    Invoke “dep status”.
	`,
}

func runInit(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Too many args: %d", len(args))
	}
	var root string
	var err error
	if len(args) == 0 {
		root, err = os.Getwd()
		if err != nil {
			return errors.Wrap(err, "os.Getwd")
		}
	} else {
		root = args[0]
	}

	mf := filepath.Join(root, manifestName)
	lf := filepath.Join(root, lockName)

	mok, err := isRegular(mf)
	if err != nil {
		return err
	}
	if mok {
		return fmt.Errorf("Manifest file %q already exists", mf)
	}
	// Manifest file does not exist.

	lok, err := isRegular(lf)
	if err != nil {
		return err
	}
	if lok {
		return fmt.Errorf("Invalid state: manifest %q does not exist, but lock %q does.", mf, lf)
	}

	cpr, err := depContext.splitAbsoluteProjectRoot(root)
	if err != nil {
		return errors.Wrap(err, "determineProjectRoot")
	}
	vlogf("Finding dependencies for %q...", cpr)
	pkgT, err := gps.ListPackages(root, cpr)
	if err != nil {
		return errors.Wrap(err, "gps.ListPackages")
	}
	vlogf("Found %d dependencies.", len(pkgT.Packages))
	sm, err := depContext.sourceManager()
	if err != nil {
		return errors.Wrap(err, "getSourceManager")
	}
	defer sm.Release()

	packages, processed, notondisk, ondisk, err := getProjectDependencies(pkgT, cpr, sm)
	if err != nil {
		return err
	}
	m := manifest{
		Dependencies: packages,
	}

	// Make an initial lock from what knowledge we've collected about the
	// versions on disk
	l := lock{
		P: make([]gps.LockedProject, 0, len(ondisk)),
	}

	for pr, v := range ondisk {
		// That we have to chop off these path prefixes is a symptom of
		// a problem in gps itself
		pkgs := make([]string, 0, len(processed[pr]))
		prslash := string(pr) + "/"
		for _, pkg := range processed[pr] {
			if pkg == string(pr) {
				pkgs = append(pkgs, ".")
			} else {
				pkgs = append(pkgs, strings.TrimPrefix(pkg, prslash))
			}
		}

		l.P = append(l.P, gps.NewLockedProject(
			gps.ProjectIdentifier{ProjectRoot: pr}, v, pkgs),
		)
	}

	var l2 *lock
	if len(notondisk) > 0 {
		vlogf("Solving...")
		params := gps.SolveParameters{
			RootDir:         root,
			RootPackageTree: pkgT,
			Manifest:        &m,
			Lock:            &l,
		}

		if *verbose {
			params.Trace = true
			params.TraceLogger = log.New(os.Stderr, "", 0)
		}
		s, err := gps.Prepare(params, sm)
		if err != nil {
			return errors.Wrap(err, "prepare solver")
		}

		soln, err := s.Solve()
		if err != nil {
			handleAllTheFailuresOfTheWorld(err)
			return err
		}
		l2 = lockFromInterface(soln)
	} else {
		l2 = &l
	}

	vlogf("Writing manifest and lock files.")
	if err := writeFile(mf, &m); err != nil {
		return errors.Wrap(err, "writeFile for manifest")
	}
	if err := writeFile(lf, l2); err != nil {
		return errors.Wrap(err, "writeFile for lock")
	}

	return nil
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

// isStdLib reports whether $GOROOT/src/path should be considered
// part of the standard distribution. For historical reasons we allow people to add
// their own code to $GOROOT instead of using $GOPATH, but we assume that
// code will start with a domain name (dot in the first element).
// This was loving taken from src/cmd/go/pkg.go in Go's code (isStandardImportPath).
func isStdLib(path string) bool {
	i := strings.Index(path, "/")
	if i < 0 {
		i = len(path)
	}
	elem := path[:i]
	return !strings.Contains(elem, ".")
}

// TODO solve failures can be really creative - we need to be similarly creative
// in handling them and informing the user appropriately
func handleAllTheFailuresOfTheWorld(err error) {
	fmt.Printf("ouchie, solve error: %s", err)
}

func writeFile(path string, in json.Marshaler) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := in.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = f.Write(b)
	return err
}

func isRegular(name string) (bool, error) {
	// TODO: lstat?
	fi, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if fi.IsDir() {
		return false, fmt.Errorf("%q is a directory, should be a file", name)
	}
	return true, nil
}

func isDir(name string) (bool, error) {
	// TODO: lstat?
	fi, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fi.IsDir() {
		return false, fmt.Errorf("%q is not a directory", name)
	}
	return true, nil
}

func hasImportPathPrefix(s, prefix string) bool {
	if s == prefix {
		return true
	}
	return strings.HasPrefix(s, prefix+"/")
}

func getProjectDependencies(pkgT gps.PackageTree, cpr string, sm *gps.SourceMgr) (dependencies gps.ProjectConstraints, processed map[gps.ProjectRoot][]string, notondisk map[gps.ProjectRoot]bool, ondisk map[gps.ProjectRoot]gps.Version, err error) {
	vlogf("Building dependency graph...")

	dependencies = make(gps.ProjectConstraints)
	processed = make(map[gps.ProjectRoot][]string)
	packages := make(map[string]bool)
	notondisk = make(map[gps.ProjectRoot]bool)
	ondisk = make(map[gps.ProjectRoot]gps.Version)
	for _, v := range pkgT.Packages {
		// TODO: Some errors maybe should not be skipped ;-)
		if v.Err != nil {
			vlogf("%v", v.Err)
			continue
		}
		vlogf("Package %q, analyzing...", v.P.ImportPath)

		for _, ip := range v.P.Imports {
			if isStdLib(ip) {
				continue
			}
			if hasImportPathPrefix(ip, cpr) {
				// Don't analyze imports from the current project.
				continue
			}
			pr, err := sm.DeduceProjectRoot(ip)
			if err != nil {
				return dependencies, processed, notondisk, ondisk, errors.Wrap(err, "sm.DeduceProjectRoot") // TODO: Skip and report ?
			}

			packages[ip] = true
			if _, ok := processed[pr]; ok {
				if !contains(processed[pr], ip) {
					processed[pr] = append(processed[pr], ip)
				}

				continue
			}
			vlogf("Package %q has import %q, analyzing...", v.P.ImportPath, ip)

			processed[pr] = []string{ip}
			v, err := depContext.versionInWorkspace(pr)
			if err != nil {
				notondisk[pr] = true
				vlogf("Could not determine version for %q, omitting from generated manifest", pr)
				continue
			}

			ondisk[pr] = v
			pp := gps.ProjectProperties{}
			switch v.Type() {
			case "branch", "version", "rev":
				pp.Constraint = v
			case "semver":
				c, _ := gps.NewSemverConstraint("^" + v.String())
				pp.Constraint = c
			}

			dependencies[pr] = pp
		}
	}

	vlogf("Analyzing transitive dependencies...")
	// Explore the packages we've found for transitive deps, either
	// completing the lock or identifying (more) missing projects that we'll
	// need to ask gps to solve for us.
	colors := make(map[string]uint8)
	const (
		white uint8 = iota
		grey
		black
	)

	// cache of PackageTrees, so we don't parse projects more than once
	ptrees := make(map[gps.ProjectRoot]gps.PackageTree)

	// depth-first traverser
	var dft func(string) error
	dft = func(pkg string) error {
		switch colors[pkg] {
		case white:
			vlogf("Analyzing %q...", pkg)
			colors[pkg] = grey

			pr, err := sm.DeduceProjectRoot(pkg)
			if err != nil {
				return errors.Wrap(err, "could not deduce project root for "+pkg)
			}

			// We already visited this project root earlier via some other
			// pkg within it, and made the decision that it's not on disk.
			// Respect that decision, and pop the stack.
			if notondisk[pr] {
				colors[pkg] = black
				return nil
			}

			ptree, has := ptrees[pr]
			if !has {
				// It's fine if the root does not exist - it indicates that this
				// project is not present in the workspace, and so we need to
				// solve to deal with this dep.
				r := filepath.Join(depContext.GOPATH, "src", string(pr))
				_, err := os.Lstat(r)
				if os.IsNotExist(err) {
					colors[pkg] = black
					notondisk[pr] = true
					return nil
				}

				ptree, err = gps.ListPackages(r, string(pr))
				if err != nil {
					// Any error here other than an a nonexistent dir (which
					// can't happen because we covered that case above) is
					// probably critical, so bail out.
					return errors.Wrap(err, "gps.ListPackages")
				}
				ptrees[pr] = ptree
			}

			rm := ptree.ExternalReach(false, false, nil)
			reached, ok := rm[pkg]
			if !ok {
				colors[pkg] = black
				// not on disk...
				notondisk[pr] = true
				return nil
			}

			if _, ok := processed[pr]; ok {
				if !contains(processed[pr], pkg) {
					processed[pr] = append(processed[pr], pkg)
				}
			} else {
				processed[pr] = []string{pkg}
			}

			// project must be on disk at this point; question is
			// whether we're first seeing it here, in the transitive
			// exploration, or if it arose in the direct dep parts
			if _, in := ondisk[pr]; !in {
				v, err := depContext.versionInWorkspace(pr)
				if err != nil {
					colors[pkg] = black
					notondisk[pr] = true
					return nil
				}
				ondisk[pr] = v
			}

			// recurse
			for _, rpkg := range reached {
				if isStdLib(rpkg) {
					continue
				}

				err := dft(rpkg)
				if err != nil {
					return err
				}
			}

			colors[pkg] = black
		case grey:
			return fmt.Errorf("Import cycle detected on %s", pkg)
		}
		return nil
	}

	// run the depth-first traversal from the set of immediate external
	// package imports we found in the current project
	for pkg := range packages {
		err := dft(pkg)
		if err != nil {
			return dependencies, processed, notondisk, ondisk, err // already errors.Wrap()'d internally
		}
	}

	return dependencies, processed, notondisk, ondisk, nil
}
