// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pkg/errors"
)

const removeShortHelp = `Remove a dependency from the project`
const removeLongHelp = `
Remove a dependency from the project's lock file, and vendor
folder. If the project includes that dependency in its import graph, remove will
fail unless -force is specified.
`

func (cmd *removeCommand) Name() string      { return "remove" }
func (cmd *removeCommand) Args() string      { return "[spec...]" }
func (cmd *removeCommand) ShortHelp() string { return removeShortHelp }
func (cmd *removeCommand) LongHelp() string  { return removeLongHelp }
func (cmd *removeCommand) Hidden() bool      { return false }

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

func (cmd *removeCommand) Run(ctx *dep.Ctx, args []string) error {
	p, err := ctx.LoadProject("")
	if err != nil {
		return err
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return err
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	cpr, err := ctx.SplitAbsoluteProjectRoot(p.AbsRoot)
	if err != nil {
		return errors.Wrap(err, "determineProjectRoot")
	}

	pkgT, err := pkgtree.ListPackages(p.AbsRoot, cpr)
	if err != nil {
		return errors.Wrap(err, "gps.ListPackages")
	}

	reachmap, _ := pkgT.ToReachMap(true, true, false, nil)

	if cmd.unused {
		if len(args) > 0 {
			return errors.Errorf("remove takes no arguments when running with -unused")
		}

		reachlist := reachmap.FlattenOmitStdLib()

		// warm the cache in parallel, in case any paths require go get metadata
		// discovery
		for _, im := range reachlist {
			go sm.DeduceProjectRoot(im)
		}

		otherroots := make(map[gps.ProjectRoot]bool)
		for _, im := range reachlist {
			if isStdLib(im) {
				continue
			}
			pr, err := sm.DeduceProjectRoot(im)
			if err != nil {
				// not being able to detect the root for an import path that's
				// actually in the import list is a deeper problem. However,
				// it's not our direct concern here, so we just warn.
				ctx.Loggers.Err.Printf("dep: could not infer root for %q\n", pr)
				continue
			}
			otherroots[pr] = true
		}

		var rm []gps.ProjectRoot
		for pr := range p.Manifest.Dependencies {
			if _, has := otherroots[pr]; !has {
				delete(p.Manifest.Dependencies, pr)
				rm = append(rm, pr)
			}
		}

		if len(rm) == 0 {
			ctx.Loggers.Err.Println("dep: nothing to do")
			return nil
		}
	} else {
		// warm the cache in parallel, in case any paths require go get metadata
		// discovery
		for _, arg := range args {
			go sm.DeduceProjectRoot(arg)
		}

		for _, arg := range args {
			pr, err := sm.DeduceProjectRoot(arg)
			if err != nil {
				// couldn't detect the project root for this string -
				// a non-valid project root was provided
				return errors.Wrap(err, "gps.DeduceProjectRoot")
			}
			if string(pr) != arg {
				// don't be magical with subpaths, otherwise we muddy the waters
				// between project roots and import paths
				return errors.Errorf("%q is not a project root, but %q is - is that what you want to remove?", arg, pr)
			}

			/*
			* - Remove package from manifest
			*	- if the package IS NOT being used, solving should do what we want
			*	- if the package IS being used:
			*		- Desired behavior: stop and tell the user, unless --force
			*		- Actual solver behavior: ?
			 */
			var pkgimport []string
			for pkg, ie := range reachmap {
				for _, im := range ie.External {
					if hasImportPathPrefix(im, arg) {
						pkgimport = append(pkgimport, pkg)
						break
					}
				}
			}

			if _, indeps := p.Manifest.Dependencies[gps.ProjectRoot(arg)]; !indeps {
				return errors.Errorf("%q is not present in the manifest, cannot remove it", arg)
			}

			if len(pkgimport) > 0 && !cmd.force {
				if len(pkgimport) == 1 {
					return errors.Errorf("not removing %q because it is imported by %q (pass -force to override)", arg, pkgimport[0])
				}
				return errors.Errorf("not removing %q because it is imported by:\n\t%s (pass -force to override)", arg, strings.Join(pkgimport, "\n\t"))
			}

			delete(p.Manifest.Dependencies, gps.ProjectRoot(arg))
		}
	}

	params := p.MakeParams()
	params.RootPackageTree = pkgT

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

	newLock := dep.LockFromSolution(soln)

	sw, err := dep.NewSafeWriter(nil, p.Lock, newLock, dep.VendorOnChanged)
	if err != nil {
		return err
	}
	if err := sw.Write(p.AbsRoot, sm, true); err != nil {
		return errors.Wrap(err, "grouped write of manifest, lock and vendor")
	}
	return nil
}
