// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"

	"github.com/golang/dep"
)

const pruneShortHelp = `Prune was merged into ensure. Use ensure instead.`
const pruneLongHelp = `
Prune was merged into the ensure command.
Set prune options in the manifest and it will be applied after every ensure.
`

type pruneCommand struct{}

func (cmd *pruneCommand) Name() string      { return "prune" }
func (cmd *pruneCommand) Args() string      { return "" }
func (cmd *pruneCommand) ShortHelp() string { return pruneShortHelp }
func (cmd *pruneCommand) LongHelp() string  { return pruneLongHelp }
func (cmd *pruneCommand) Hidden() bool      { return true }

func (cmd *pruneCommand) Register(fs *flag.FlagSet) {}

func (cmd *pruneCommand) Run(ctx *dep.Ctx, args []string) error {
	ctx.Out.Printf("Prune was merged into ensure.\n")
	ctx.Out.Printf("Set prune settings in %s and it it will be applied when running ensure.\n", dep.ManifestName)

	return nil
}
