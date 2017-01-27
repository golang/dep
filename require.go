// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"log"
	"os"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

func (cmd *requireCommand) Name() string { return "require" }
func (cmd *requireCommand) Args() string { return "" }
func (cmd *requireCommand) ShortHelp() string {
	return "Require a package to be vendored even if it is not imported."
}
func (cmd *requireCommand) LongHelp() string { return "" }
func (cmd *requireCommand) Hidden() bool     { return false }

func (cmd *requireCommand) Register(fs *flag.FlagSet) {
}

type requireCommand struct{}

func (_ requireCommand) Run(args []string) error {
	if len(args) == 0 {
		return errors.New("must pass a package to require")
	}
	p, err := depContext.loadProject("")
	if err != nil {
		return err
	}

	sm, err := depContext.sourceManager()
	if err != nil {
		return err
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	pt, err := gps.ListPackages(p.absroot, string(p.importroot))
	if err != nil {
		return errors.Wrap(err, "require ListPackage for project")
	}

	reachMap := pt.ExternalReach(true, true, p.m.IgnoredPackages())
	external := reachMap.ListExternalImports()
	em := map[string]bool{}
	for _, ep := range external {
		em[ep] = true
	}

	curRequired := map[string]bool{}
	for _, rp := range p.m.Required {
		curRequired[rp] = true
	}

	for _, arg := range args {
		if _, err := sm.DeduceProjectRoot(arg); err != nil {
			return errors.Wrapf(err, "could not deduce project root for %s", arg)
		}

		if curRequired[arg] {
			logf("%s is already required, skipping", arg)
			continue
		}

		if em[arg] {
			logf("%s is imported, adding to required.", arg)
		}

		// add to manifest
		p.m.Required = append(p.m.Required, arg)
	}

	params := p.makeParams()
	params.RootPackageTree = pt
	if *verbose {
		params.Trace = true
		params.TraceLogger = log.New(os.Stderr, "", 0)
	}

	solver, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "require Prepare")
	}

	sw := safeWriter{
		root: p.absroot,
		m:    p.m,
		sm:   sm,
	}

	if bytes.Equal(solver.HashInputs(), p.l.InputHash()) {
		return errors.Wrap(sw.writeAllSafe(false), "writing of manifest")
	}

	solution, err := solver.Solve()
	if err != nil {
		handleAllTheFailuresOfTheWorld(err)
		return errors.Wrap(err, "require Solve()")
	}
	sw.l = p.l
	sw.nl = solution

	return errors.Wrap(sw.writeAllSafe(false), "grouped write of manifest, lock and vendor")
}
