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
)

func main() {
	flag.Parse()

	// newContext() will set the GOPATH for us to use for various functions.
	depContext = newContext()

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

// The following command declarations should be moved to their own files as
// they are implemented.

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

// loadProject searches for a project root from the provided path, then loads
// the manifest and lock (if any) it finds there.
//
// If the provided path is empty, it will search from the path indicated by
// os.Getwd().
func (c *ctx) loadProject(path string) (*project, error) {
	var err error
	p := new(project)

	switch path {
	case "":
		p.absroot, err = findProjectRootFromWD()
	default:
		p.absroot, err = findProjectRoot(path)
	}

	if err != nil {
		return p, err
	}

	var match bool
	for _, gp := range filepath.SplitList(c.GOPATH) {
		srcprefix := filepath.Join(gp, "src") + string(filepath.Separator)
		if strings.HasPrefix(p.absroot, srcprefix) {
			match = true
			// filepath.ToSlash because we're dealing with an import path now,
			// not an fs path
			p.importroot = gps.ProjectRoot(filepath.ToSlash(strings.TrimPrefix(p.absroot, srcprefix)))
			break
		}
	}
	if !match {
		return nil, fmt.Errorf("could not determine project root - not on GOPATH")
	}

	mp := filepath.Join(p.absroot, manifestName)
	mf, err := os.Open(mp)
	if err != nil {
		if os.IsNotExist(err) {
			// TODO: list possible solutions? (dep init, cd $project)
			return nil, fmt.Errorf("no %v found in project root %v", manifestName, p.absroot)
		}
		// Unable to read the manifest file
		return nil, err
	}
	defer mf.Close()

	p.m, err = readManifest(mf)
	if err != nil {
		return nil, fmt.Errorf("error while parsing %s: %s", mp, err)
	}

	lp := filepath.Join(path, lockName)
	lf, err := os.Open(lp)
	if err != nil {
		if os.IsNotExist(err) {
			// It's fine for the lock not to exist
			return p, nil
		}
		// But if a lock does exist and we can't open it, that's a problem
		return nil, fmt.Errorf("could not open %s: %s", lp, err)
	}

	defer lf.Close()
	p.l, err = readLock(lf)
	if err != nil {
		return nil, fmt.Errorf("error while parsing %s: %s", lp, err)
	}

	return p, nil
}
