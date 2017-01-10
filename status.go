// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/sdboyer/gps"
)

const statusShortHelp = `Report the status of the project's dependencies`
const statusLongHelp = `
With no arguments, print the status of each dependency of the project.

  PROJECT     Import path
  CONSTRAINT  Version constraint, from the manifest
  VERSION     Version chosen, from the lock
  REVISION    VCS revision of the chosen version
  LATEST      Latest VCS revision available
  PKGS USED   Number of packages from this project that are actually used

With one or more explicitly specified packages, or with the -detailed flag,
print an extended status output for each dependency of the project.

  TODO    Another column description
  FOOBAR  Another column description

Status returns exit code zero if all dependencies are in a "good state".
`

func (cmd *statusCommand) Name() string      { return "status" }
func (cmd *statusCommand) Args() string      { return "[package...]" }
func (cmd *statusCommand) ShortHelp() string { return statusShortHelp }
func (cmd *statusCommand) LongHelp() string  { return statusLongHelp }

func (cmd *statusCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.detailed, "detailed", false, "report more detailed status")
	fs.BoolVar(&cmd.json, "json", false, "output in JSON format")
	fs.StringVar(&cmd.template, "f", "", "output in text/template format")
	fs.BoolVar(&cmd.dot, "dot", false, "output the dependency graph in GraphViz format")
	fs.BoolVar(&cmd.old, "old", false, "only show out-of-date dependencies")
	fs.BoolVar(&cmd.missing, "missing", false, "only show missing dependencies")
	fs.BoolVar(&cmd.unused, "unused", false, "only show unused dependencies")
	fs.BoolVar(&cmd.modified, "modified", false, "only show modified dependencies")
}

type statusCommand struct {
	detailed bool
	json     bool
	template string
	dot      bool
	old      bool
	missing  bool
	unused   bool
	modified bool
}

func (cmd *statusCommand) Run(args []string) error {
	p, err := depContext.loadProject("")
	if err != nil {
		return err
	}

	sm, err := depContext.sourceManager()
	if err != nil {
		return err
	}
	defer sm.Release()

	if cmd.detailed {
		return runStatusDetailed(p, sm, args)
	}
	return runStatusAll(p, sm)
}

// BasicStatus contains all the information reported about a single dependency
// in the summary/list status output mode.
type BasicStatus struct {
	ProjectRoot  string
	Constraint   gps.Constraint
	Version      gps.UnpairedVersion
	Revision     gps.Revision
	Latest       gps.Version
	PackageCount int
}

type MissingStatus struct {
	ProjectRoot     string
	MissingPackages string
}

func runStatusAll(p *project, sm *gps.SourceMgr) error {
	if p.l == nil {
		// TODO if we have no lock file, do...other stuff
		return nil
	}

	// While the network churns on ListVersions() requests, statically analyze
	// code from the current project.
	ptree, err := gps.ListPackages(p.absroot, string(p.importroot))
	if err != nil {
		return fmt.Errorf("analysis of local packages failed: %v", err)
	}

	// Set up a solver in order to check the InputHash.
	params := gps.SolveParameters{
		RootDir:         p.absroot,
		RootPackageTree: ptree,
		Manifest:        p.m,
		// Locks aren't a part of the input hash check, so we can omit it.
	}
	if *verbose {
		params.Trace = true
		params.TraceLogger = log.New(os.Stderr, "", 0)
	}

	s, err := gps.Prepare(params, sm)
	if err != nil {
		return fmt.Errorf("could not set up solver for input hashing: %s", err)
	}

	cm := collectConstraints(ptree, p, sm)

	// Get the project list and sort it so that the printed output users see is
	// deterministically ordered. (This may be superfluous if the lock is always
	// written in alpha order, but it doesn't hurt to double down.)
	slp := p.l.Projects()
	sort.Sort(sortedLockedProjects(slp))

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	defer tw.Flush()

	if bytes.Equal(s.HashInputs(), p.l.Memo) {
		// If these are equal, we're guaranteed that the lock is a transitively
		// complete picture of all deps. That eliminates the need for at least
		// some checks.

		fmt.Fprintf(tw, "PROJECT\tCONSTRAINT\tVERSION\tREVISION\tLATEST\tPKGS USED\n")

		for _, proj := range slp {
			bs := BasicStatus{
				ProjectRoot:  string(proj.Ident().ProjectRoot),
				PackageCount: len(proj.Packages()),
			}

			// Split apart the version from the lock into its constituent parts
			switch tv := proj.Version().(type) {
			case gps.UnpairedVersion:
				bs.Version = tv
			case gps.Revision:
				bs.Revision = tv
			case gps.PairedVersion:
				bs.Version = tv.Unpair()
				bs.Revision = tv.Underlying()
			}

			// Check if the manifest has an override for this project. If so,
			// set that as the constraint.
			if pp, has := p.m.Ovr[proj.Ident().ProjectRoot]; has && pp.Constraint != nil {
				// TODO note somehow that it's overridden
				bs.Constraint = pp.Constraint
			} else {
				bs.Constraint = gps.Any()
				for _, c := range cm[bs.ProjectRoot] {
					bs.Constraint = c.Intersect(bs.Constraint)
				}
			}

			// Only if we have a non-rev and non-plain version do/can we display
			// anything wrt the version's updateability.
			if bs.Version != nil && bs.Version.Type() != gps.IsVersion {
				c, has := p.m.Dependencies[proj.Ident().ProjectRoot]
				if !has {
					c.Constraint = gps.Any()
				}
				// TODO: This constraint is only the constraint imposed by the
				// current project, not by any transitive deps. As a result,
				// transitive project deps will always show "any" here.
				bs.Constraint = c.Constraint

				vl, err := sm.ListVersions(proj.Ident())
				if err == nil {
					gps.SortForUpgrade(vl)

					for _, v := range vl {
						// Because we've sorted the version list for
						// upgrade, the first version we encounter that
						// matches our constraint will be what we want.
						if c.Constraint.Matches(v) {
							// For branch constraints this should be the
							// most recent revision on the selected
							// branch.
							if tv, ok := v.(gps.PairedVersion); ok && v.Type() == gps.IsBranch {
								bs.Latest = tv.Underlying()
							} else {
								bs.Latest = v
							}
							break
						}
					}
				}
			}

			var constraint string
			if v, ok := bs.Constraint.(gps.Version); ok {
				constraint = formatVersion(v)
			} else {
				constraint = bs.Constraint.String()
			}

			fmt.Fprintf(tw,
				"%s\t%s\t%s\t%s\t%s\t%d\t\n",
				bs.ProjectRoot,
				constraint,
				formatVersion(bs.Version),
				formatVersion(bs.Revision),
				formatVersion(bs.Latest),
				bs.PackageCount,
			)
		}

		return nil
	}

	// Hash digest mismatch may indicate that some deps are no longer
	// needed, some are missing, or that some constraints or source
	// locations have changed.
	//
	// It's possible for digests to not match, but still have a correct
	// lock.
	fmt.Fprintf(tw, "PROJECT\tMISSING PACKAGES\n")

	external := ptree.ExternalReach(true, false, nil).ListExternalImports()
	roots := make(map[gps.ProjectRoot][]string)
	var errs []string
	for _, e := range external {
		root, err := sm.DeduceProjectRoot(e)
		if err != nil {
			errs = append(errs, string(root))
			continue
		}

		roots[root] = append(roots[root], e)
	}

outer:
	for root, pkgs := range roots {
		// TODO also handle the case where the project is present, but there
		// are items missing from just the package list
		for _, lp := range slp {
			if lp.Ident().ProjectRoot == root {
				continue outer
			}
		}

		fmt.Fprintf(tw,
			"%s\t%s\t\n",
			root,
			pkgs,
		)
	}

	return nil
}

func formatVersion(v gps.Version) string {
	if v == nil {
		return ""
	}
	switch v.Type() {
	case gps.IsBranch:
		return "branch " + v.String()
	case gps.IsRevision:
		r := v.String()
		if len(r) > 7 {
			r = r[:7]
		}
		return r
	}
	return v.String()
}

func runStatusDetailed(p *project, sm *gps.SourceMgr, args []string) error {
	// TODO
	return fmt.Errorf("not implemented")
}

func collectConstraints(ptree gps.PackageTree, p *project, sm *gps.SourceMgr) map[string][]gps.Constraint {
	// TODO
	return map[string][]gps.Constraint{}
}
