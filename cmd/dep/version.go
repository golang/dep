// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"

	"github.com/golang/dep"
)

const versionShortHelp = `Display version`
const versionLongHelp = `
Display version of this application.
`

const Version = "0.0.1"

func (cmd *versionCommand) Name() string      { return "version" }
func (cmd *versionCommand) Args() string      { return "" }
func (cmd *versionCommand) ShortHelp() string { return versionShortHelp }
func (cmd *versionCommand) LongHelp() string  { return versionLongHelp }
func (cmd *versionCommand) Hidden() bool      { return false }

func (cmd *versionCommand) Register(fs *flag.FlagSet) {
}

type versionCommand struct {
}

func (cmd *versionCommand) Run(ctx *dep.Ctx, args []string) error {
	fmt.Println(Version)
	return nil
}
