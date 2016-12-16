// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func main() {
	flag.Usage = func() {
		help(nil)
	}
	flag.Parse()

	// newContext() will set the GOPATH for us to use for various functions.
	var err error
	depContext, err = newContext()
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}

	do := flag.Arg(0)
	var args []string
	if do == "" {
		do = "help"
	} else {
		args = flag.Args()
	}
	for _, cmd := range commands {
		if do != cmd.name {
			continue
		}

		if cmd.flag != nil {
			cmd.flag.Usage = func() { cmd.Usage() }
			err = cmd.flag.Parse(args[1:])
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
			}
			args = cmd.flag.Args()
		} else {
			if len(args) > 0 {
				args = args[1:]
			}
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
	flag  *flag.FlagSet
}

func (c *command) Usage() {
	fmt.Fprintf(os.Stderr, "usage: %s\n\n", c.short)
	fmt.Fprintf(os.Stderr, "%s\n", strings.TrimSpace(c.long))
	os.Exit(2)
}

var commands = []*command{
	initCmd,
	statusCmd,
	ensureCmd,
	versionCmd,
	// help added here at init time.
}

var (
	errProjectNotFound = errors.New("no project could be found")
)

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
