// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command dep is a prototype dependency management tool.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/golang/dep"
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
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get working directory", err)
		os.Exit(1)
	}
	c := &Config{
		Args:       os.Args,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		WorkingDir: wd,
		Env:        os.Environ(),
	}
	os.Exit(c.Run())
}

// A Config specifies a full configuration for a dep execution.
type Config struct {
	WorkingDir     string    // Where to execute
	Args           []string  // Command-line arguments, starting with the program name.
	Env            []string  // Environment variables
	Stdout, Stderr io.Writer // Log output
}

// Run executes a configuration and returns an exit code.
func (c *Config) Run() (exitCode int) {
	// Build the list of available commands.
	commands := []command{
		&initCommand{},
		&statusCommand{},
		&ensureCommand{},
		&hashinCommand{},
		&pruneCommand{},
		&versionCommand{},
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
			"dep ensure -add github.com/pkg/errors",
			"add a dependency to the project",
		},
	}

	outLogger := log.New(c.Stdout, "", 0)
	errLogger := log.New(c.Stderr, "", 0)

	usage := func() {
		errLogger.Println("dep is a tool for managing dependencies for Go projects")
		errLogger.Println()
		errLogger.Println("Usage: dep <command>")
		errLogger.Println()
		errLogger.Println("Commands:")
		errLogger.Println()
		w := tabwriter.NewWriter(c.Stderr, 0, 4, 2, ' ', 0)
		for _, cmd := range commands {
			if !cmd.Hidden() {
				fmt.Fprintf(w, "\t%s\t%s\n", cmd.Name(), cmd.ShortHelp())
			}
		}
		w.Flush()
		errLogger.Println()
		errLogger.Println("Examples:")
		for _, example := range examples {
			fmt.Fprintf(w, "\t%s\t%s\n", example[0], example[1])
		}
		w.Flush()
		errLogger.Println()
		errLogger.Println("Use \"dep help [command]\" for more information about a command.")
	}

	cmdName, printCommandHelp, exit := parseArgs(c.Args)
	if exit {
		usage()
		exitCode = 1
		return
	}

	for _, cmd := range commands {
		if cmd.Name() == cmdName {
			// Build flag set with global flags in there.
			fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
			fs.SetOutput(c.Stderr)
			verbose := fs.Bool("v", false, "enable verbose logging")

			// Register the subcommand flags in there, too.
			cmd.Register(fs)

			// Override the usage text to something nicer.
			resetUsage(errLogger, fs, cmdName, cmd.Args(), cmd.LongHelp())

			if printCommandHelp {
				fs.Usage()
				exitCode = 1
				return
			}

			// Parse the flags the user gave us.
			// flag package automatically prints usage and error message in err != nil
			// or if '-h' flag provided
			if err := fs.Parse(c.Args[2:]); err != nil {
				exitCode = 1
				return
			}

			// Set up dep context.
			ctx := &dep.Ctx{
				Out:     outLogger,
				Err:     errLogger,
				Verbose: *verbose,
			}

			GOPATHS := filepath.SplitList(getEnv(c.Env, "GOPATH"))
			ctx.SetPaths(c.WorkingDir, GOPATHS...)

			// Run the command with the post-flag-processing args.
			if err := cmd.Run(ctx, fs.Args()); err != nil {
				errLogger.Printf("%v\n", err)
				exitCode = 1
				return
			}

			// Easy peasy livin' breezy.
			return
		}
	}

	errLogger.Printf("dep: %s: no such command\n", cmdName)
	usage()
	exitCode = 1
	return
}

func resetUsage(logger *log.Logger, fs *flag.FlagSet, name, args, longHelp string) {
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
		logger.Printf("Usage: dep %s %s\n", name, args)
		logger.Println()
		logger.Println(strings.TrimSpace(longHelp))
		logger.Println()
		if hasFlags {
			logger.Println("Flags:")
			logger.Println()
			logger.Println(flagBlock.String())
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

// getEnv returns the last instance of an environment variable.
func getEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		v := env[i]
		kv := strings.SplitN(v, "=", 2)
		if kv[0] == key {
			if len(kv) > 1 {
				return kv[1]
			}
			return ""
		}
	}
	return ""
}
