// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

const (
	manifestName = "manifest.json"
	lockName     = "lock.json"
)

var (
	depContext *ctx
	verbose    = flag.Bool("v", false, "enable verbose logging")
)

type command interface {
	Name() string           // "foobar"
	Args() string           // "<baz> [quux...]"
	ShortHelp() string      // "Foo the first bar"
	LongHelp() string       // "Foo the first bar meeting the following conditions..."
	Register(*flag.FlagSet) // command-specific flags
	Hidden() bool           // indicates whether the command should be hidden from help output
	Run([]string) error
}

func main() {
	// Set up the dep context.
	// TODO(pb): can this be deglobalized, pretty please?
	hc, err := newContext()
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
	depContext = hc

	// Build the list of available commands.
	commands := []command{
		&initCommand{},
		&statusCommand{},
		&ensureCommand{},
		&removeCommand{},
		&requireCommand{},
		&hashinCommand{},
	}

	usage := func() {
		fmt.Fprintln(os.Stderr, "Usage: dep <command>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr)
		w := tabwriter.NewWriter(os.Stderr, 0, 4, 2, ' ', 0)
		for _, command := range commands {
			if !command.Hidden() {
				fmt.Fprintf(w, "\t%s\t%s\n", command.Name(), command.ShortHelp())
			}
		}
		w.Flush()
		fmt.Fprintln(os.Stderr)
	}

	if len(os.Args) <= 1 || len(os.Args) == 2 && (strings.Contains(strings.ToLower(os.Args[1]), "help") || strings.ToLower(os.Args[1]) == "-h") {
		usage()
		os.Exit(1)
	}

	for _, command := range commands {
		if name := command.Name(); os.Args[1] == name {
			// Build flag set with global flags in there.
			// TODO(pb): can we deglobalize verbose, pretty please?
			fs := flag.NewFlagSet(name, flag.ExitOnError)
			fs.BoolVar(verbose, "v", false, "enable verbose logging")

			// Register the subcommand flags in there, too.
			command.Register(fs)

			// Override the usage text to something nicer.
			resetUsage(fs, command.Name(), command.Args(), command.LongHelp())

			// Parse the flags the user gave us.
			if err := fs.Parse(os.Args[2:]); err != nil {
				fs.Usage()
				os.Exit(1)
			}

			// Run the command with the post-flag-processing args.
			if err := command.Run(fs.Args()); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			// Easy peasy livin' breezy.
			return
		}
	}

	fmt.Fprintf(os.Stderr, "%s: no such command\n", os.Args[1])
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

var (
	errProjectNotFound = errors.New("could not find project manifest.json")
)

func findProjectRootFromWD() (string, error) {
	path, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get working directory: %s", err)
	}
	return findProjectRoot(path)
}

// search upwards looking for a manifest file until we get to the root of the
// filesystem
func findProjectRoot(from string) (string, error) {
	for {
		mp := filepath.Join(from, manifestName)

		_, err := os.Stat(mp)
		if err == nil {
			return from, nil
		}
		if !os.IsNotExist(err) {
			// Some err other than non-existence - return that out
			return "", err
		}

		parent := filepath.Dir(from)
		if parent == from {
			return "", errProjectNotFound
		}
		from = parent
	}
}

type project struct {
	// absroot is the absolute path to the root directory of the project.
	absroot string
	// importroot is the import path of the project's root directory.
	importroot gps.ProjectRoot
	m          *manifest
	l          *lock
}

// makeParams is a simple helper to create a gps.SolveParameters without setting
// any nils incorrectly.
func (p *project) makeParams() gps.SolveParameters {
	params := gps.SolveParameters{
		RootDir: p.absroot,
	}

	if p.m != nil {
		params.Manifest = p.m
	}

	if p.l != nil {
		params.Lock = p.l
	}

	return params
}

func logf(format string, args ...interface{}) {
	// TODO: something else?
	fmt.Fprintf(os.Stderr, "dep: "+format+"\n", args...)
}

func vlogf(format string, args ...interface{}) {
	if !*verbose {
		return
	}
	logf(format, args...)
}
