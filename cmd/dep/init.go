// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/paths"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pkg/errors"
)

const initShortHelp = `Initialize a new project with manifest and lock files`
const initLongHelp = `
Initialize the project at filepath root by parsing its dependencies, writing
manifest and lock files, and vendoring the dependencies. If root isn't
specified, use the current directory.

When configuration for another dependency management tool is detected, it is
imported into the initial manifest and lock. Use the -skip-tools flag to
disable this behavior. The following external tools are supported: glide, godep.
Any dependencies that are not constrained by external configuration use the
GOPATH analysis below.

By default, the dependencies are resolved over the network. A version will be
selected from the versions available from the upstream source per the following
algorithm:

 - Tags conforming to semver (sorted by semver rules)
 - Default branch(es) (sorted lexicographically)
 - Non-semver tags (sorted lexicographically)

An alternate mode can be activated by passing -gopath. In this mode, the version
of each dependency will reflect the current state of the GOPATH. If a dependency
doesn't exist in the GOPATH, a version will be selected based on the above
network version selection algorithm.

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
	fs.BoolVar(&cmd.skipTools, "skip-tools", false, "skip importing configuration from other dependency managers")
	fs.BoolVar(&cmd.gopath, "gopath", false, "search in GOPATH for dependencies")
}

type initCommand struct {
	noExamples bool
	skipTools  bool
	gopath     bool
}

func (cmd *initCommand) Run(ctx *dep.Ctx, args []string) error {
	if len(args) > 1 {
		return errors.Errorf("too many args (%d)", len(args))
	}

	var root string
	if len(args) <= 0 {
		// Set project root to current working directory.
		root = ctx.WorkingDir
	} else {
		root = args[0]
		if !filepath.IsAbs(args[0]) {
			root = filepath.Join(ctx.WorkingDir, args[0])
		}
		if err := os.MkdirAll(root, os.FileMode(0777)); err != nil {
			return errors.Errorf("unable to create directory %s , err %v", root, err)
		}
	}

	var err error
	p := new(dep.Project)
	if err = p.SetRoot(root); err != nil {
		return errors.Wrap(err, "NewProject")
	}

	ctx.GOPATH, err = ctx.DetectProjectGOPATH(p)
	if err != nil {
		return errors.Wrapf(err, "ctx.DetectProjectGOPATH")
	}

	mf := filepath.Join(root, dep.ManifestName)
	lf := filepath.Join(root, dep.LockName)
	vpath := filepath.Join(root, "vendor")

	mok, err := fs.IsRegular(mf)
	if err != nil {
		return err
	}
	if mok {
		return errors.Errorf("manifest already exists: %s", mf)
	}

	lok, err := fs.IsRegular(lf)
	if err != nil {
		return err
	}
	if lok {
		return errors.Errorf("invalid state: manifest %q does not exist, but lock %q does", mf, lf)
	}

	// A manifest file does not exist in the current working directory.
	// We want to search up the directory tree and warn if present.
	// On attempts to load the project, ignore errors returned by ctx.LoadProject()
	// and perform a nil-check on the project root returned.
	pr, _ := ctx.LoadProject()
	if pr != nil {
		// TODO: Defining an error type `NoRootProjectFound` would allow for explicit checks against
		// this type, handling it specifically. Thoughts?
		ctx.Out.Println("WARNING: found manifest file in another directory.")
	}

	// Warn if new project initialization is being performed in a project subdirectory.
	subdir, err := checkInSubdir(ctx)
	if err != nil {
		return errors.Wrap(err, "checkInSubDir")
	}
	if subdir {
		ctx.Out.Println("WARNING: it is recommended that project initialization be performed at the project root, not a project subdirectory.")
	}

	ip, err := ctx.ImportForAbs(root)
	if err != nil {
		return errors.Wrap(err, "root project import")
	}
	p.ImportRoot = gps.ProjectRoot(ip)

	pkgT, directDeps, err := getDirectDependencies(p)
	if err != nil {
		return err
	}
	sm, err := ctx.SourceManager()
	if err != nil {
		return errors.Wrap(err, "getSourceManager")
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	// Initialize with imported data, then fill in the gaps using the GOPATH
	rootAnalyzer := newRootAnalyzer(cmd.skipTools, ctx, directDeps, sm)
	p.Manifest, p.Lock, err = rootAnalyzer.InitializeRootManifestAndLock(root, p.ImportRoot)
	if err != nil {
		return err
	}

	if cmd.gopath {
		gs := newGopathScanner(ctx, directDeps, sm)
		err = gs.InitializeRootManifestAndLock(p.Manifest, p.Lock)
		if err != nil {
			return err
		}
	}

	rootAnalyzer.skipTools = true // Don't import external config during solve for now
	copyLock := *p.Lock           // Copy lock before solving. Use this to separate new lock projects from solved lock

	params := gps.SolveParameters{
		RootDir:         root,
		RootPackageTree: pkgT,
		Manifest:        p.Manifest,
		Lock:            p.Lock,
		ProjectAnalyzer: rootAnalyzer,
	}

	if ctx.Verbose {
		params.TraceLogger = ctx.Err
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
	p.Lock = dep.LockFromSolution(soln)

	rootAnalyzer.FinalizeRootManifestAndLock(p.Manifest, p.Lock, copyLock)

	// Run gps.Prepare with appropriate constraint solutions from solve run
	// to generate the final lock memo.
	s, err = gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "prepare solver")
	}

	p.Lock.SolveMeta.InputsDigest = s.HashInputs()

	// Pass timestamp (yyyyMMddHHmmss format) as suffix to backup name.
	vendorbak, err := dep.BackupVendor(vpath, time.Now().Format("20060102150405"))
	if err != nil {
		return err
	}
	if vendorbak != "" {
		ctx.Err.Printf("Old vendor backed up to %v", vendorbak)
	}

	sw, err := dep.NewSafeWriter(p.Manifest, nil, p.Lock, dep.VendorAlways)
	if err != nil {
		return err
	}

	if err := sw.Write(root, sm, !cmd.noExamples); err != nil {
		return errors.Wrap(err, "safe write of manifest and lock")
	}

	return nil
}

// checkInSubdir returns true if project initialization is being performed in a subdirectory
// relative to the project root directory `$GOPATH/src/github.com/user/project`; otherwise returns false.
func checkInSubdir(ctx *dep.Ctx) (bool, error) {
	sr, err := ctx.ImportForAbs(ctx.WorkingDir)
	if err != nil {
		return false, err
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return false, err
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	pr, err := sm.DeduceProjectRoot(sr)
	if err != nil {
		return false, err
	}

	// TODO: This is unlikely the cleanest way to determine if we are in a subdirectory
	// of the project root path.
	in := !strings.HasSuffix(ctx.WorkingDir, string(pr))
	return in, nil
}

func getDirectDependencies(p *dep.Project) (pkgtree.PackageTree, map[string]bool, error) {
	pkgT, err := pkgtree.ListPackages(p.ResolvedAbsRoot, string(p.ImportRoot))
	if err != nil {
		return pkgtree.PackageTree{}, nil, errors.Wrap(err, "gps.ListPackages")
	}

	directDeps := map[string]bool{}
	rm, _ := pkgT.ToReachMap(true, true, false, nil)
	for _, pr := range rm.FlattenFn(paths.IsStandardImportPath) {
		directDeps[pr] = true
	}

	return pkgT, directDeps, nil
}

// TODO solve failures can be really creative - we need to be similarly creative
// in handling them and informing the user appropriately
func handleAllTheFailuresOfTheWorld(err error) {
}
