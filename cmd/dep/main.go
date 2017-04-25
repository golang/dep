// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command dep is a prototype dependency management tool.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/golang/dep"
	"github.com/golang/dep/internal"
)

var (
	verbose = flag.Bool("v", false, "enable verbose logging")
)

type command interface {
	Name() string           // "foobar"
	Args() string           // "<baz> [quux...]"
	ShortHelp() string      // "Foo the first bar"
	LongHelp() string       // "Foo the first bar meeting the following conditions..."
	Register(*flag.FlagSet) // command-specific flags
	Hidden() bool           // indicates whether the command should be hidden from help output
	Run(*dep.Ctx, []string) error
}

func main() {
	// Build the list of available commands.
	commands := []command{
		&initCommand{},
		&statusCommand{},
		&ensureCommand{},
		&removeCommand{},
		&hashinCommand{},
		&pruneCommand{},
	}

	examples := [][2]string{
		{
			"dep init",
			"set up a new project",
		},
		{
			"dep ensure",
			"install the project's dependencies",
		},
		{
			"dep ensure -update",
			"update the locked versions of all dependencies",
		},
		{
			"dep ensure github.com/pkg/errors",
			"add a dependency to the project",
		},
	}

	usage := func() {
		fmt.Fprintln(os.Stderr, "dep is a tool for managing dependencies for Go projects")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Usage: dep <command>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr)
		w := tabwriter.NewWriter(os.Stderr, 0, 4, 2, ' ', 0)
		for _, cmd := range commands {
			if !cmd.Hidden() {
				fmt.Fprintf(w, "\t%s\t%s\n", cmd.Name(), cmd.ShortHelp())
			}
		}
		w.Flush()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		for _, example := range examples {
			fmt.Fprintf(w, "\t%s\t%s\n", example[0], example[1])
		}
		w.Flush()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Use \"dep help [command]\" for more information about a command.")
	}

	cmdName, printCommandHelp, exit := parseArgs(os.Args)
	if exit {
		usage()
		os.Exit(1)
	}

	for _, cmd := range commands {
		if cmd.Name() == cmdName {
			// Build flag set with global flags in there.
			// TODO(pb): can we deglobalize verbose, pretty please?
			fs := flag.NewFlagSet(cmdName, flag.ExitOnError)
			fs.BoolVar(verbose, "v", false, "enable verbose logging")

			// Register the subcommand flags in there, too.
			cmd.Register(fs)

			// Override the usage text to something nicer.
			resetUsage(fs, cmdName, cmd.Args(), cmd.LongHelp())

			if printCommandHelp {
				fs.Usage()
				os.Exit(1)
			}

			// Parse the flags the user gave us.
			if err := fs.Parse(os.Args[2:]); err != nil {
				fs.Usage()
				os.Exit(1)
			}

			internal.Verbose = *verbose

			// Set up the dep context.
			ctx, err := dep.NewContext()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			// Run the command with the post-flag-processing args.
			if err := cmd.Run(ctx, fs.Args()); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			// Easy peasy livin' breezy.
			return
		}
	}

	fmt.Fprintf(os.Stderr, "%s: no such command\n", cmdName)
	usage()
	os.Exit(1)
}

func resetUsage(fs *flag.FlagSet, name, args, longHelp string) {
	var (
		hasFlags   bool
		flagBlock  bytes.Buffer
		flagWriter = tabwriter.NewWriter(&flagBlock, 0, 4, 2, ' ', 0)
	)
	fs.VisitAll(func(f *flag.Flag) {
		hasFlags = true
		// Default-empty string vars should read "(default: <none>)"
		// rather than the comparatively ugly "(default: )".
		defValue := f.DefValue
		if defValue == "" {
			defValue = "<none>"
		}
		fmt.Fprintf(flagWriter, "\t-%s\t%s (default: %s)\n", f.Name, f.Usage, defValue)
	})
	flagWriter.Flush()
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dep %s %s\n", name, args)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, strings.TrimSpace(longHelp))
		fmt.Fprintln(os.Stderr)
		if hasFlags {
			fmt.Fprintln(os.Stderr, "Flags:")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, flagBlock.String())
		}
	}
}

// parseArgs determines the name of the dep command and whether the user asked for
// help to be printed.
func parseArgs(args []string) (cmdName string, printCmdUsage bool, exit bool) {
	isHelpArg := func() bool {
		return strings.Contains(strings.ToLower(args[1]), "help") || strings.ToLower(args[1]) == "-h"
	}

	switch len(args) {
	case 0, 1:
		exit = true
	case 2:
		if isHelpArg() {
			exit = true
		}
		cmdName = args[1]
	default:
		if isHelpArg() {
			cmdName = args[2]
			printCmdUsage = true
		} else {
			cmdName = args[1]
		}
	}
	return cmdName, printCmdUsage, exit
}
