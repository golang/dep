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
func (cmd *hashinCommand) Hidden() bool      { return false }

func (cmd *hashinCommand) Register(fs *flag.FlagSet) {
}

type hashinCommand struct{}

func (_ hashinCommand) Run(args []string) error {
	p, err := hoardContext.loadProject("")
	if err != nil {
		return err
	}

	sm, err := hoardContext.sourceManager()
	if err != nil {
		return err
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	params := p.makeParams()
	cpr, err := hoardContext.splitAbsoluteProjectRoot(p.absroot)
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
