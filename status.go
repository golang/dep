package main

import (
	"bytes"
	"fmt"

	"github.com/sdboyer/gps"
)

var statusCmd = &command{
	fn:   noop,
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
	- VCS state (uncommitted changes? pruned?)

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
	} else {
		return runStatusDetailed(p, sm, args)
	}
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
		go func() {
			sm.ListVersions(id)
		}()
	}

	// Statically analyze code from the current project while the network churns
	ptree, err := gps.ListPackages(p.root, string(p.pr))

	// Set up a solver in order to check the InputHash.
	params := gps.SolveParameters{
		RootDir:         rd,
		RootPackageTree: rt,
		Manifest:        conf,
		// Locks aren't a part of the input hash check, so we can omit it.
	}

	s, err = gps.Prepare(params, sm)
	if err != nil {
		return fmt.Errorf("could not set up solver for input hashing, err: %s", err)
	}
	digest := s.HashInputs()
	if bytes.Equal(digest, p.l.Memo) {
		// If these are equal, we're guaranteed that the lock is a transitively
		// complete picture of all deps. That may change the output we need to
		// generate
	} else {
		// Not equal - the lock may or may not be a complete picture, and even
		// if it does have all the deps, it may not be a valid set of
		// constraints.
	}

	return nil
}

func runStatusDetailed(p *project, sm *gps.SourceMgr, args []string) error {
	// TODO
	return fmt.Errorf("not implemented")
}
