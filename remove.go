// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

const removeShortHelp = `Remove a dependency from the project`
const removeLongHelp = `
Remove a dependency from the project's manifest file, lock file, and vendor
folder. If the project includes that dependency in its import graph, remove will
fail unless -force is specified.
`

func (cmd *removeCommand) Name() string      { return "remove" }
func (cmd *removeCommand) Args() string      { return "[spec...]" }
func (cmd *removeCommand) ShortHelp() string { return removeShortHelp }
func (cmd *removeCommand) LongHelp() string  { return removeLongHelp }

func (cmd *removeCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.dryRun, "n", false, "dry run, don't actually remove anything")
	fs.BoolVar(&cmd.unused, "unused", false, "remove all dependencies that aren't imported by the project")
	fs.BoolVar(&cmd.force, "force", false, "remove the given dependencies even if they are imported by the project")
	fs.BoolVar(&cmd.keepSource, "keep-source", false, "don't remove source code")
}

type removeCommand struct {
	dryRun     bool
	unused     bool
	force      bool
	keepSource bool
}

func (cmd *removeCommand) Run(args []string) error {
	p, err := depContext.loadProject("")
	if err != nil {
		return err
	}

	sm, err := depContext.sourceManager()
	if err != nil {
		return err
	}
	defer sm.Release()

	cpr, err := depContext.splitAbsoluteProjectRoot(p.absroot)
	if err != nil {
		return errors.Wrap(err, "determineProjectRoot")
	}

	pkgT, err := gps.ListPackages(p.absroot, cpr)
	if err != nil {
		return errors.Wrap(err, "gps.ListPackages")
	}

	// get the list of packages
	pd, err := getProjectData(pkgT, cpr, sm)
	if err != nil {
		return err
	}

	for _, arg := range args {
		/*
		 * - Remove package from manifest
		 *	- if the package IS NOT being used, solving should do what we want
		 *	- if the package IS being used:
		 *		- Desired behavior: stop and tell the user, unless --force
		 *		- Actual solver behavior: ?
		 */

		if _, found := pd.dependencies[gps.ProjectRoot(arg)]; found {
			//TODO: Tell the user where it is in use?
			return fmt.Errorf("not removing '%s' because it is in use", arg)
		}
		delete(p.m.Dependencies, gps.ProjectRoot(arg))
	}

	params := gps.SolveParameters{
		RootDir:         p.absroot,
		RootPackageTree: pkgT,
		Manifest:        p.m,
		Lock:            p.l,
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

	p.l = lockFromInterface(soln)

	if err := writeFile(filepath.Join(p.absroot, manifestName), p.m); err != nil {
		return errors.Wrap(err, "writeFile for manifest")
	}
	if err := writeFile(filepath.Join(p.absroot, lockName), p.l); err != nil {
		return errors.Wrap(err, "writeFile for lock")
	}
	return nil
}
