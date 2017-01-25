// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

func (cmd *hashinCommand) Name() string      { return "hash-inputs" }
func (cmd *hashinCommand) Args() string      { return "" }
func (cmd *hashinCommand) ShortHelp() string { return "" }
func (cmd *hashinCommand) LongHelp() string  { return "" }
func (cmd *hashinCommand) Hidden() bool      { return true }

func (cmd *hashinCommand) Register(fs *flag.FlagSet) {}

type hashinCommand struct{}

func (hashinCommand) Run(args []string) error {
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

	params := p.makeParams()
	cpr, err := depContext.splitAbsoluteProjectRoot(p.absroot)
	if err != nil {
		return errors.Wrap(err, "determineProjectRoot")
	}

	params.RootPackageTree, err = gps.ListPackages(p.absroot, cpr)
	if err != nil {
		return errors.Wrap(err, "gps.ListPackages")
	}

	s, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "prepare solver")
	}

	fmt.Println(gps.HashingInputsAsString(s))
	return nil
}
