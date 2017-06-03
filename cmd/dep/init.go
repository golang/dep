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
disable this behavior. The following external tools are supported: glide.
Any dependencies that are not constrained by external configuration use the
GOPATH analysis below.

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
	fs.BoolVar(&cmd.skipTools, "skip-tools", false, "skip importing configuration from other dependency managers")
}

type initCommand struct {
	noExamples bool
	skipTools  bool
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
		if !filepath.IsAbs(args[0]) {
			root = filepath.Join(ctx.WorkingDir, args[0])
		}
		if err := os.MkdirAll(root, os.FileMode(0777)); err != nil {
			return errors.Errorf("unable to create directory %s , err %v", root, err)
		}
	}

	// The root path may lie within a symlinked directory, resolve the path
	// before moving forward
	var err error
	root, ctx.GOPATH, err = ctx.ResolveProjectRootAndGoPath(root)
	if err != nil {
		return errors.Wrapf(err, "resolve project root")
	} else if ctx.GOPATH == "" {
		return errors.New("project not within a GOPATH")
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
	// Manifest file does not exist.

	lok, err := fs.IsRegular(lf)
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
	pkgT, directDeps, err := getDirectDependencies(root, cpr)
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
	m, l, err := rootAnalyzer.InitializeRootManifestAndLock(root, gps.ProjectRoot(cpr))
	if err != nil {
		return err
	}
	gs := newGopathScanner(ctx, directDeps, sm)
	err = gs.InitializeRootManifestAndLock(m, l)
	if err != nil {
		return err
	}

	rootAnalyzer.skipTools = true // Don't import external config during solve for now

	params := gps.SolveParameters{
		RootDir:         root,
		RootPackageTree: pkgT,
		Manifest:        m,
		Lock:            l,
		ProjectAnalyzer: rootAnalyzer,
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
	l = dep.LockFromSolution(soln)

	rootAnalyzer.FinalizeRootManifestAndLock(m, l)
	gs.FinalizeRootManifestAndLock(m, l)

	// Run gps.Prepare with appropriate constraint solutions from solve run
	// to generate the final lock memo.
	s, err = gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "prepare solver")
	}

	l.SolveMeta.InputsDigest = s.HashInputs()

	// Pass timestamp (yyyyMMddHHmmss format) as suffix to backup name.
	vendorbak, err := dep.BackupVendor(vpath, time.Now().Format("20060102150405"))
	if err != nil {
		return err
	}
	if vendorbak != "" {
		ctx.Loggers.Err.Printf("Old vendor backed up to %v", vendorbak)
	}

	sw, err := dep.NewSafeWriter(m, nil, l, dep.VendorAlways)
	if err != nil {
		return err
	}

	if err := sw.Write(root, sm, !cmd.noExamples); err != nil {
		return errors.Wrap(err, "safe write of manifest and lock")
	}

	return nil
}

func getDirectDependencies(root, cpr string) (pkgtree.PackageTree, map[string]bool, error) {
	pkgT, err := pkgtree.ListPackages(root, cpr)
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

func hasImportPathPrefix(s, prefix string) bool {
	if s == prefix {
		return true
	}
	return strings.HasPrefix(s, prefix+"/")
}
