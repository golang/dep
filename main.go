// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Parse()

	do := flag.Arg(0)
	var args []string
	if do == "" {
		do = "help"
	} else {
		args = flag.Args()[1:]
	}
	for _, cmd := range commands {
		if do != cmd.name {
			continue
		}
		if err := cmd.fn(args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "unknown command: %q", flag.Arg(0))
	help(nil)
	os.Exit(2)
}

type command struct {
	fn    func(args []string) error
	name  string
	short string
	long  string
}

var commands = []*command{
	initCmd,
	statusCmd,
	getCmd,
	// help added here at init time.
}

func init() {
	// Defeat circular declarations by appending
	// this to the list at init time.
	commands = append(commands, &command{
		fn:   help,
		name: "help",
		short: `[command]
	Show documentation for the dep tool or the specified command.
	`,
	})
}

func help(args []string) error {
	if len(args) > 1 {
		// If they're misusing help, show them how it's done.
		args = []string{"help"}
	}
	if len(args) == 0 {
		// Show short usage for all commands.
		fmt.Printf("usage: dep <command> [arguments]\n\n")
		fmt.Printf("Available commands:\n\n")
		for _, cmd := range commands {
			fmt.Printf("%s %s\n", cmd.name, cmd.short)
		}
		return nil
	}
	// Show full help for a specific command.
	for _, cmd := range commands {
		if cmd.name != args[0] {
			continue
		}
		fmt.Printf("usage: dep %s %s%s\n", cmd.name, cmd.short, cmd.long)
		return nil
	}
	return fmt.Errorf("unknown command: %q", args[0])
}

func noop(args []string) error {
	fmt.Println("noop called with flags:", args)
	return nil
}

// The following command declarations should be moved to their own files as
// they are implemented.

var initCmd = &command{
	fn:   noop,
	name: "init",
	short: `
	Write Manifest file in the root of the project directory.
	`,
	long: `
	Populates Manifest file with current deps of this project. 
	The specified version of each dependent repository is the version
	available in the user's workspaces (as specified by GOPATH).
	If the dependency is not present in any workspaces it is not be
	included in the Manifest.
	Writes Lock file(?)
	Creates vendor/ directory(?)
	`,
}

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

var getCmd = &command{
	fn:   noop,
	name: "get",
	short: `[flags] [package specs]
	Fetch or update dependencies.
	`,
	long: `
	Flags:
		-a	update all packages
		-x	dry run
		-f	force the given package to be updated to the specified
			version
		
	Package specs:
		<path>[@<version specifier>]

	Ignores (? architectures / tags )
		-t	(?) include tests

	Destinations (?)
		-vendor	(?) get to workspace or vendor directory
	`,
}
