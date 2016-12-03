// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/sdboyer/gps"
)

var statusCmd = &command{
	fn:   runStatus,
	name: "status",
	short: `[flags] [packages]
	Report the status of the current project's dependencies.
	`,
	long: `
	If no packages are specified, for each dependency:
	- root import path
	- (if present in lock) the currently selected version
	- (else) that it's missing from the lock
	- whether it's present in the vendor directory (or if it's in
	  workspace, if that's a thing?)
	- the current aggregate constraints on that project (as specified by
	  the Manifest)
	- if -u is specified, whether there are newer versions of this
	  dependency

	If packages are specified, or if -a is specified,
	for each of those dependencies:
	- (if present in lock) the currently selected version
	- (else) that it's missing from the lock
	- whether it's present in the vendor directory
	- The set of possible versions for that project
	- The upstream source URL(s) from which the project may be retrieved
	- The type of upstream source (git, hg, bzr, svn, registry)
	- Other versions that might work, given the current constraints
	- The list of all projects that import the project within the current
	  depgraph
	- The current constraint. If more than one project constrains it, both
	  the aggregate and the individual components (and which project provides
	  that constraint) are printed
	- License information
	- Package source location, if fetched from an alternate location

	Flags:
	-json		Output in json format
	-f [template]	Output in text/template format

	-old		Only show out of date packages and the current version
	-missing	Only show missing packages.
	-unused		Only show unused packages.
	-modified	Only show modified packages.

	-dot		Export dependency graph in GraphViz format

	The exit code of status is zero if all repositories are in a "good state".
	`,
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

func runStatus(args []string) error {
	p, err := loadProject("")
	if err != nil {
		return err
	}

	sm, err := getSourceManager()
	if err != nil {
		return err
	}
	defer sm.Release()

	if len(args) == 0 {
		return runStatusAll(p, sm)
	}
	return runStatusDetailed(p, sm, args)
}

func runStatusAll(p *project, sm *gps.SourceMgr) error {
	if p.l == nil {
		// TODO if we have no lock file, do...other stuff
		return nil
	}

	// In the background, warm caches of version lists for all the projects in
	// the lock. The SourceMgr coordinates access to this information - if the
	// main goroutine asks for the version list while the background goroutine's
	// request is in flight (or vice versa), both calls are folded together and
	// are fulfilled from the same network response, and the same on-disk
	// repository cache.
	for _, proj := range p.l.Projects() {
		id := proj.Ident()
		go sm.ListVersions(id)
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

	if bytes.Equal(s.HashInputs(), p.l.Memo) {
		// If these are equal, we're guaranteed that the lock is a transitively
		// complete picture of all deps. That eliminates the need for at least
		// some checks.

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 1, ' ', 0)
		fmt.Fprintf(tw, "Project\tConstraint\tVersion\tRevision\tLatest\tPkgs Used\t\n")

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
			if bs.Version != nil && bs.Version.Type() != "version" {
				c, has := p.m.Dependencies[proj.Ident().ProjectRoot]
				if !has {
					c.Constraint = gps.Any()
				}

				vl, err := sm.ListVersions(proj.Ident())
				if err != nil {
					gps.SortForUpgrade(vl)

					for _, v := range vl {
						// Because we've sorted the version list for upgrade, the
						// first version we encounter that matches our constraint
						// will be what we want
						if c.Constraint.Matches(v) {
							bs.Latest = v
							break
						}
					}
				}
			}

			fmt.Fprintf(tw,
				"%s\t%s\t%s\t%s\t%s\t%d\t\n",
				bs.ProjectRoot,
				bs.Constraint,
				bs.Version,
				string(bs.Revision)[:7],
				bs.Latest,
				bs.PackageCount,
			)
		}

		tw.Flush()
	} else {
		// Hash digest mismatch may indicate that some deps are no longer
		// needed, some are missing, or that some constraints or source
		// locations have changed.
		//
		// It's possible for digests to not match, but still have a correct
		// lock.
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 1, ' ', 0)
		fmt.Fprintf(tw, "Project\tMissing Packages\t\n")

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
		tw.Flush()
	}

	return nil
}

type sortedLockedProjects []gps.LockedProject

func (s sortedLockedProjects) Len() int      { return len(s) }
func (s sortedLockedProjects) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortedLockedProjects) Less(i, j int) bool {
	l, r := s[i].Ident(), s[j].Ident()

	if l.ProjectRoot < r.ProjectRoot {
		return true
	}
	if r.ProjectRoot < l.ProjectRoot {
		return false
	}

	return l.NetworkName < r.NetworkName
}

func runStatusDetailed(p *project, sm *gps.SourceMgr, args []string) error {
	// TODO
	return fmt.Errorf("not implemented")
}

func collectConstraints(ptree gps.PackageTree, p *project, sm *gps.SourceMgr) map[string][]gps.Constraint {
	// TODO
	return map[string][]gps.Constraint{}
}
