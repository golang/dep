// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/dep"
	"github.com/golang/dep/internal"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/golang/dep/internal/paths"
	"github.com/pkg/errors"
)

const initShortHelp = `Initialize a new project with manifest and lock files`
const initLongHelp = `
Initialize the project at filepath root by parsing its dependencies, writing
manifest and lock files, and vendoring the dependencies. If root isn't
specified, use the current directory.

The version of each dependency will reflect the current state of the GOPATH. If
a dependency doesn't exist in the GOPATH, a version will be selected from the
versions available from the upstream source per the following algorithm:

 - Tags conforming to semver (sorted by semver rules)
 - Default branch(es) (sorted lexicographically)
 - Non-semver tags (sorted lexicographically)

A Gopkg.toml file will be written with inferred version constraints for all
direct dependencies. Gopkg.lock will be written with precise versions, and
vendor/ will be populated with the precise versions written to Gopkg.lock.
`

func (cmd *initCommand) Name() string      { return "init" }
func (cmd *initCommand) Args() string      { return "[root]" }
func (cmd *initCommand) ShortHelp() string { return initShortHelp }
func (cmd *initCommand) LongHelp() string  { return initLongHelp }
func (cmd *initCommand) Hidden() bool      { return false }

func (cmd *initCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.noExamples, "no-examples", false, "don't include example in Gopkg.toml")
}

type initCommand struct {
	noExamples bool
}

func trimPathPrefix(p1, p2 string) string {
	if internal.HasFilepathPrefix(p1, p2) {
		return p1[len(p2):]
	}
	return p1
}

func (cmd *initCommand) Run(ctx *dep.Ctx, args []string) error {
	if len(args) > 1 {
		return errors.Errorf("too many args (%d)", len(args))
	}

	var root string
	if len(args) <= 0 {
		root = ctx.WorkingDir
	} else {
		root = args[0]
	}

	mf := filepath.Join(root, dep.ManifestName)
	lf := filepath.Join(root, dep.LockName)
	vpath := filepath.Join(root, "vendor")

	mok, err := dep.IsRegular(mf)
	if err != nil {
		return err
	}
	if mok {
		return errors.Errorf("manifest already exists: %s", mf)
	}
	// Manifest file does not exist.

	lok, err := dep.IsRegular(lf)
	if err != nil {
		return err
	}
	if lok {
		return errors.Errorf("invalid state: manifest %q does not exist, but lock %q does", mf, lf)
	}

	cpr, err := ctx.SplitAbsoluteProjectRoot(root)
	if err != nil {
		return errors.Wrap(err, "determineProjectRoot")
	}
	if ctx.Loggers.Verbose {
		ctx.Loggers.Err.Printf("dep: Finding dependencies for %q...\n", cpr)
	}
	pkgT, err := pkgtree.ListPackages(root, cpr)
	if err != nil {
		return errors.Wrap(err, "gps.ListPackages")
	}
	if ctx.Loggers.Verbose {
		ctx.Loggers.Err.Printf("dep: Found %d dependencies.\n", len(pkgT.Packages))
	}
	sm, err := ctx.SourceManager()
	if err != nil {
		return errors.Wrap(err, "getSourceManager")
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	ctx.Loggers.Err.Println("Searching GOPATH for projects...")
	pd, err := getProjectData(ctx, pkgT, cpr, sm)
	if err != nil {
		return err
	}
	m := &dep.Manifest{
		Dependencies: pd.constraints,
	}

	// Make an initial lock from what knowledge we've collected about the
	// versions on disk
	l := &dep.Lock{
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
				pkgs = append(pkgs, trimPathPrefix(pkg, prslash))
			}
		}

		l.P = append(l.P, gps.NewLockedProject(
			gps.ProjectIdentifier{ProjectRoot: pr}, v, pkgs),
		)
	}

	ctx.Loggers.Err.Println("Using network for remaining projects...")
	// Copy lock before solving. Use this to separate new lock projects from soln
	copyLock := *l

	// Run solver with project versions found on disk
	if ctx.Loggers.Verbose {
		ctx.Loggers.Err.Println("dep: Solving...")
	}
	params := gps.SolveParameters{
		RootDir:         root,
		RootPackageTree: pkgT,
		Manifest:        m,
		Lock:            l,
		ProjectAnalyzer: dep.Analyzer{},
	}

	if ctx.Loggers.Verbose {
		params.TraceLogger = ctx.Loggers.Err
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
	l = dep.LockFromInterface(soln)

	// Iterate through the new projects in solved lock and add them to manifest
	// if direct deps and log feedback for all the new projects.
	for _, x := range l.Projects() {
		pr := x.Ident().ProjectRoot
		newProject := true
		// Check if it's a new project, not in the old lock
		for _, y := range copyLock.Projects() {
			if pr == y.Ident().ProjectRoot {
				newProject = false
			}
		}
		if newProject {
			// Check if it's in notondisk project map. These are direct deps, should
			// be added to manifest.
			if _, ok := pd.notondisk[pr]; ok {
				m.Dependencies[pr] = getProjectPropertiesFromVersion(x.Version())
				feedback(x.Version(), pr, fb.DepTypeDirect, ctx)
			} else {
				// Log feedback of transitive project
				feedback(x.Version(), pr, fb.DepTypeTransitive, ctx)
			}
		}
	}

	// Run gps.Prepare with appropriate constraint solutions from solve run
	// to generate the final lock memo.
	s, err = gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "prepare solver")
	}

	l.Memo = s.HashInputs()

	// Pass timestamp (yyyyMMddHHmmss format) as suffix to backup name.
	vendorbak, err := dep.BackupVendor(vpath, time.Now().Format("20060102150405"))
	if err != nil {
		return err
	}
	if vendorbak != "" {
		ctx.Loggers.Err.Printf("Old vendor backed up to %v", vendorbak)
	}

	if ctx.Loggers.Verbose {
		ctx.Loggers.Err.Println("dep: Writing manifest and lock files.")
	}

	sw, err := dep.NewSafeWriter(m, nil, l, dep.VendorAlways)
	if err != nil {
		return err
	}

	if err := sw.Write(root, sm, cmd.noExamples); err != nil {
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

// TODO solve failures can be really creative - we need to be similarly creative
// in handling them and informing the user appropriately
func handleAllTheFailuresOfTheWorld(err error) {
}

func hasImportPathPrefix(s, prefix string) bool {
	if s == prefix {
		return true
	}
	return strings.HasPrefix(s, prefix+"/")
}

// feedback logs project constraint as feedback to the user.
func feedback(v gps.Version, pr gps.ProjectRoot, depType string, ctx *dep.Ctx) {
	rev, version, branch := gps.VersionComponentStrings(v)

	// Check if it's a valid SHA1 digest and trim to 7 characters.
	if len(rev) == 40 {
		if _, err := hex.DecodeString(rev); err == nil {
			// Valid SHA1 digest
			rev = rev[0:7]
		}
	}

	// Get LockedVersion
	var ver string
	if version != "" {
		ver = version
	} else if branch != "" {
		ver = branch
	}

	cf := &fb.ConstraintFeedback{
		LockedVersion:  ver,
		Revision:       rev,
		ProjectPath:    string(pr),
		DependencyType: depType,
	}

	// Get non-revision constraint if available
	if c := getProjectPropertiesFromVersion(v).Constraint; c != nil {
		cf.Version = c.String()
	}

	// Attach ConstraintType for direct dep based on locked version
	if cf.DependencyType == fb.DepTypeDirect {
		if cf.LockedVersion != "" {
			cf.ConstraintType = fb.ConsTypeConstraint
		} else {
			cf.ConstraintType = fb.ConsTypeHint
		}
	}

	cf.LogFeedback(ctx)
}

// getProjectPropertiesFromVersion takes a gps.Version and returns a proper
// gps.ProjectProperties with Constraint value based on the provided version.
func getProjectPropertiesFromVersion(v gps.Version) gps.ProjectProperties {
	pp := gps.ProjectProperties{}

	// extract version and ignore if it's revision only
	switch tv := v.(type) {
	case gps.PairedVersion:
		v = tv.Unpair()
	case gps.Revision:
		return pp
	}

	switch v.Type() {
	case gps.IsBranch, gps.IsVersion:
		pp.Constraint = v
	case gps.IsSemver:
		// TODO: remove "^" when https://github.com/golang/dep/issues/225 is ready.
		c, err := gps.NewSemverConstraint("^" + v.String())
		if err != nil {
			panic(err)
		}
		pp.Constraint = c
	}

	return pp
}

type projectData struct {
	constraints  gps.ProjectConstraints          // constraints that could be found
	dependencies map[gps.ProjectRoot][]string    // all dependencies (imports) found by project root
	notondisk    map[gps.ProjectRoot]bool        // projects that were not found on disk
	ondisk       map[gps.ProjectRoot]gps.Version // projects that were found on disk
}

func getProjectData(ctx *dep.Ctx, pkgT pkgtree.PackageTree, cpr string, sm gps.SourceManager) (projectData, error) {
	constraints := make(gps.ProjectConstraints)
	dependencies := make(map[gps.ProjectRoot][]string)
	packages := make(map[string]bool)
	notondisk := make(map[gps.ProjectRoot]bool)
	ondisk := make(map[gps.ProjectRoot]gps.Version)

	var syncDepGroup sync.WaitGroup
	syncDep := func(pr gps.ProjectRoot, sm gps.SourceManager) {
		if err := sm.SyncSourceFor(gps.ProjectIdentifier{ProjectRoot: pr}); err != nil {
			ctx.Loggers.Err.Printf("Unable to cache %s", pr)
		}
		syncDepGroup.Done()
	}

	rm, _ := pkgT.ToReachMap(true, true, false, nil)
	if len(rm) == 0 {
		return projectData{}, nil
	}

	if ctx.Loggers.Verbose {
		ctx.Loggers.Err.Println("dep: Building dependency graph...")
	}
	for _, ip := range rm.FlattenOmitStdLib() {
		pr, err := sm.DeduceProjectRoot(ip)
		if err != nil {
			return projectData{}, errors.Wrap(err, "sm.DeduceProjectRoot") // TODO: Skip and report ?
		}

		packages[ip] = true
		if _, has := dependencies[pr]; has {
			dependencies[pr] = append(dependencies[pr], ip)
			continue
		}
		syncDepGroup.Add(1)
		go syncDep(pr, sm)

		if ctx.Loggers.Verbose {
			ctx.Loggers.Err.Printf("dep: Found import of %q, analyzing...\n", ip)
		}

		dependencies[pr] = []string{ip}
		v, err := ctx.VersionInWorkspace(pr)
		if err != nil {
			notondisk[pr] = true
			if ctx.Loggers.Verbose {
				ctx.Loggers.Err.Printf("dep: Could not determine version for %q, omitting from generated manifest\n", pr)
			}
			continue
		}

		ondisk[pr] = v
		pp := getProjectPropertiesFromVersion(v)
		if pp.Constraint != nil || pp.Source != "" {
			constraints[pr] = pp
		}

		feedback(v, pr, fb.DepTypeDirect, ctx)
	}

	if ctx.Loggers.Verbose {
		ctx.Loggers.Err.Printf("dep: Analyzing transitive imports...\n")
	}
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
	ptrees := make(map[gps.ProjectRoot]pkgtree.PackageTree)

	// depth-first traverser
	var dft func(string) error
	dft = func(pkg string) error {
		switch colors[pkg] {
		case white:
			if ctx.Loggers.Verbose {
				ctx.Loggers.Err.Printf("dep: Analyzing %q...\n", pkg)
			}
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
				r := filepath.Join(ctx.GOPATH, "src", string(pr))
				fi, err := os.Stat(r)
				if os.IsNotExist(err) || !fi.IsDir() {
					colors[pkg] = black
					notondisk[pr] = true
					return nil
				}

				// We know the project is on disk; the question is whether we're
				// first seeing it here, in the transitive exploration, or if it
				// was found in the initial pass on direct imports. We know it's
				// the former if there's no entry for it in the ondisk map.
				if _, in := ondisk[pr]; !in {
					v, err := ctx.VersionInWorkspace(pr)
					if err != nil {
						// Even if we know it's on disk, errors are still
						// possible when trying to deduce version. If we
						// encounter such an error, just treat the project as
						// not being on disk; the solver will work it out.
						colors[pkg] = black
						notondisk[pr] = true
						return nil
					}
					ondisk[pr] = v
					feedback(v, pr, fb.DepTypeTransitive, ctx)
				}

				ptree, err = pkgtree.ListPackages(r, string(pr))
				if err != nil {
					// Any error here other than an a nonexistent dir (which
					// can't happen because we covered that case above) is
					// probably critical, so bail out.
					return errors.Wrap(err, "gps.ListPackages")
				}
				ptrees[pr] = ptree
			}

			// Get a reachmap that includes main pkgs (even though importing
			// them is an error, what we're checking right now is simply whether
			// there's a package with go code present on disk), and does not
			// backpropagate errors (again, because our only concern right now
			// is package existence).
			rm, errmap := ptree.ToReachMap(true, false, false, nil)
			reached, ok := rm[pkg]
			if !ok {
				colors[pkg] = black
				// not on disk...
				notondisk[pr] = true
				return nil
			}
			if _, ok := errmap[pkg]; ok {
				// The package is on disk, but contains some errors.
				colors[pkg] = black
				return nil
			}

			if deps, has := dependencies[pr]; has {
				if !contains(deps, pkg) {
					dependencies[pr] = append(deps, pkg)
				}
			} else {
				dependencies[pr] = []string{pkg}
				syncDepGroup.Add(1)
				go syncDep(pr, sm)
			}

			// recurse
			for _, rpkg := range reached.External {
				if paths.IsStandardImportPath(rpkg) {
					continue
				}

				err := dft(rpkg)
				if err != nil {
					// Bubble up any errors we encounter
					return err
				}
			}

			colors[pkg] = black
		case grey:
			return errors.Errorf("Import cycle detected on %s", pkg)
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

	syncDepGroup.Wait()

	pd := projectData{
		constraints:  constraints,
		dependencies: dependencies,
		notondisk:    notondisk,
		ondisk:       ondisk,
	}
	return pd, nil
}
