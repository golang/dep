// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

const initShortHelp = `Initialize a new project with manifest and lock files`
const initLongHelp = `
Initialize the project at filepath root by parsing its dependencies and writing
manifest and lock files. If root isn't specified, use the current directory.

The version of each dependency will reflect the current state of the GOPATH. If
a dependency doesn't exist in the GOPATH, it won't be written to the manifest,
but it will be solved-for, and will appear in the lock.

Note: init may use the network to solve the dependency graph.

Note: init does NOT vendor dependencies at the moment. See dep ensure.
`

func (cmd *initCommand) Name() string      { return "init" }
func (cmd *initCommand) Args() string      { return "[root]" }
func (cmd *initCommand) ShortHelp() string { return initShortHelp }
func (cmd *initCommand) LongHelp() string  { return initLongHelp }

func (cmd *initCommand) Register(fs *flag.FlagSet) {}

type initCommand struct{}

func (cmd *initCommand) Run(args []string) error {
	if len(args) > 1 {
		return errors.Errorf("too many args (%d)", len(args))
	}

	var root string
	if len(args) <= 0 {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = wd
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
		return fmt.Errorf("manifest file %q already exists", mf)
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

	pd, err := getProjectData(pkgT, cpr, sm)
	if err != nil {
		return err
	}
	m := manifest{
		Dependencies: pd.constraints,
	}

	// Make an initial lock from what knowledge we've collected about the
	// versions on disk
	l := lock{
		P: make([]gps.LockedProject, 0, len(pd.ondisk)),
	}

	for pr, v := range pd.ondisk {
		// That we have to chop off these path prefixes is a symptom of
		// a problem in gps itself
		pkgs := make([]string, 0, len(pd.dependencies[pr]))
		prslash := string(pr) + "/"
		for _, pkg := range pd.dependencies[pr] {
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

	sw := safeWriter{
		root: root,
		sm:   sm,
		m:    &m,
	}

	if len(pd.notondisk) > 0 {
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
		sw.l = lockFromInterface(soln)
	} else {
		sw.l = &l
	}

	vlogf("Writing manifest and lock files.")

	if err := sw.writeAllSafe(false); err != nil {
		return errors.Wrap(err, "safe write of manifest and lock")
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

type projectData struct {
	constraints  gps.ProjectConstraints          // constraints that could be found
	dependencies map[gps.ProjectRoot][]string    // all dependencies (imports) found by project root
	notondisk    map[gps.ProjectRoot]bool        // projects that were not found on disk
	ondisk       map[gps.ProjectRoot]gps.Version // projects that were found on disk
}

func getProjectData(pkgT gps.PackageTree, cpr string, sm *gps.SourceMgr) (projectData, error) {
	vlogf("Building dependency graph...")

	constraints := make(gps.ProjectConstraints)
	dependencies := make(map[gps.ProjectRoot][]string)
	packages := make(map[string]bool)
	notondisk := make(map[gps.ProjectRoot]bool)
	ondisk := make(map[gps.ProjectRoot]gps.Version)
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
				return projectData{}, errors.Wrap(err, "sm.DeduceProjectRoot") // TODO: Skip and report ?
			}

			packages[ip] = true
			if _, ok := dependencies[pr]; ok {
				if !contains(dependencies[pr], ip) {
					dependencies[pr] = append(dependencies[pr], ip)
				}

				continue
			}
			vlogf("Package %q has import %q, analyzing...", v.P.ImportPath, ip)

			dependencies[pr] = []string{ip}
			v, err := depContext.versionInWorkspace(pr)
			if err != nil {
				notondisk[pr] = true
				vlogf("Could not determine version for %q, omitting from generated manifest", pr)
				continue
			}

			ondisk[pr] = v
			pp := gps.ProjectProperties{}
			switch v.Type() {
			case gps.IsBranch, gps.IsVersion, gps.IsRevision:
				pp.Constraint = v
			case gps.IsSemver:
				c, _ := gps.NewSemverConstraint("^" + v.String())
				pp.Constraint = c
			}

			constraints[pr] = pp
		}
	}

	vlogf("Analyzing transitive imports...")
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

			if _, ok := dependencies[pr]; ok {
				if !contains(dependencies[pr], pkg) {
					dependencies[pr] = append(dependencies[pr], pkg)
				}
			} else {
				dependencies[pr] = []string{pkg}
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
			return projectData{}, err // already errors.Wrap()'d internally
		}
	}

	pd := projectData{
		constraints:  constraints,
		dependencies: dependencies,
		notondisk:    notondisk,
		ondisk:       ondisk,
	}
	return pd, nil
}
