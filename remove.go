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
	"strings"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

var removeCmd = &command{
	fn:   runRemove,
	name: "rm",
	flag: flag.NewFlagSet("", flag.ExitOnError),
	short: `[flags] [packages]
	Remove a package or a set of packages.
	`,
	long: `
Run it when:
To stop using dependencies
To clean out unused dependencies

What it does
Removes the given dependency from the Manifest, Lock, and vendor/.
If the current project includes that dependency in its import graph, rm will fail unless -force is specified.
If -unused is provided, specs matches all dependencies in the Manifest that are not reachable by the import graph.
The -force and -unused flags cannot be combined (an error occurs).
During removal, dependencies that were only present because of the dependencies being removed are also removed.

Note: this is a separate command to 'ensure' because we want the user to be explicit when making destructive changes.

Flags:
-n		Dry run, don’t actually remove anything
-unused	Remove dependencies that are not used by this project
-force		Remove dependency even if it is used by the project
-keep-source	Do not remove source code
	`,
}

var unused, force bool

func init() {
	removeCmd.flag.BoolVar(&force, "force", false, "Remove dependencies from manifest even if present in the current project's import")
	removeCmd.flag.BoolVar(&unused, "unused", false, "Purge all dependencies from manifest that are no longer imported by the project")
}

func runRemove(args []string) error {
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

	// TODO this will end up ignoring internal pkgs with errs (and any other
	// internal pkgs that import them), which is not what we want for this mode.
	// A new callback, or a new param on this one, will be introduced to gps
	// soon, and we'll want to use that when it arrives.
	//reachlist := pkgT.ExternalReach(true, true).ListExternalImports()
	reachmap := pkgT.ExternalReach(true, true, nil)

	if unused {
		if len(args) > 0 {
			return fmt.Errorf("rm takes no arguments when running with -unused")
		}

		reachlist := reachmap.ListExternalImports()

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
				logf("could not infer root for %q", pr)
				continue
			}
			otherroots[pr] = true
		}

		var rm []gps.ProjectRoot
		for pr, _ := range p.m.Dependencies {
			if _, has := otherroots[pr]; !has {
				delete(p.m.Dependencies, pr)
				rm = append(rm, pr)
			}
		}

		if len(rm) == 0 {
			logf("nothing to do")
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
				return fmt.Errorf("%q is not a project root, but %q is - is that what you want to remove?", arg, pr)
			}

			/*
			* - Remove package from manifest
			*	- if the package IS NOT being used, solving should do what we want
			*	- if the package IS being used:
			*		- Desired behavior: stop and tell the user, unless --force
			*		- Actual solver behavior: ?
			 */
			var pkgimport []string
			for pkg, imports := range reachmap {
				for _, im := range imports {
					if hasImportPathPrefix(im, arg) {
						pkgimport = append(pkgimport, pkg)
						break
					}
				}
			}

			if _, indeps := p.m.Dependencies[gps.ProjectRoot(arg)]; !indeps {
				return fmt.Errorf("%q is not present in the manifest, cannot remove it", arg)
			}

			if len(pkgimport) > 0 && !force {
				if len(pkgimport) == 1 {
					return fmt.Errorf("not removing %q because it is imported by %q (pass -force to override)", arg, pkgimport[0])
				} else {
					return fmt.Errorf("not removing %q because it is imported by:\n\t%s (pass -force to override)", arg, strings.Join(pkgimport, "\n\t"))
				}
			}

			delete(p.m.Dependencies, gps.ProjectRoot(arg))
		}
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
