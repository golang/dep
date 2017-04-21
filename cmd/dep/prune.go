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

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/pkgtree"

	"github.com/pkg/errors"
)

const pruneShortHelp = `Prune the vendor tree of unused packages`
const pruneLongHelp = `
Prune is used to remove unused packages from your vendor tree.

STABILITY NOTICE: this command creates problems for vendor/ verification. As
such, it may be removed and/or moved out into a separate project later on.
`

type pruneCommand struct {
}

func (cmd *pruneCommand) Name() string      { return "prune" }
func (cmd *pruneCommand) Args() string      { return "" }
func (cmd *pruneCommand) ShortHelp() string { return pruneShortHelp }
func (cmd *pruneCommand) LongHelp() string  { return pruneLongHelp }
func (cmd *pruneCommand) Hidden() bool      { return false }

func (cmd *pruneCommand) Register(fs *flag.FlagSet) {
}

func (cmd *pruneCommand) Run(ctx *dep.Ctx, args []string) error {
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

	// While the network churns on ListVersions() requests, statically analyze
	// code from the current project.
	ptree, err := pkgtree.ListPackages(p.AbsRoot, string(p.ImportRoot))
	if err != nil {
		return errors.Wrap(err, "analysis of local packages failed: %v")
	}

	// Set up a solver in order to check the InputHash.
	params := gps.SolveParameters{
		RootDir:         p.AbsRoot,
		RootPackageTree: ptree,
		Manifest:        p.Manifest,
		// Locks aren't a part of the input hash check, so we can omit it.
	}
	if *verbose {
		params.Trace = true
		params.TraceLogger = log.New(os.Stderr, "", 0)
	}

	s, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "could not set up solver for input hashing")
	}

	if !bytes.Equal(s.HashInputs(), p.Lock.Memo) {
		return fmt.Errorf("lock hash doesn't match")
	}

	return dep.PruneProject(p, sm)
}
