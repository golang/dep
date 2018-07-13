// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/verify"
	"github.com/pkg/errors"
)

const checkShortHelp = `Check if imports, Gopkg.toml, and Gopkg.lock are in sync`
const checkLongHelp = `
Check determines if your project is in a good state. If problems are found, it
prints a description of each issue, then exits 1. Passing -q suppresses output.

Flags control which specific checks will be run. By default, dep check verifies
that Gopkg.lock is in sync with Gopkg.toml and the imports in your project's .go
files, and that the vendor directory is in sync with Gopkg.lock. These checks
can be disabled with -skip-lock and -skip-vendor, respectively.

(See https://golang.github.io/dep/docs/ensure-mechanics.html#staying-in-sync for
more information on what it means to be "in sync.")
`

type checkCommand struct {
	quiet                bool
	skiplock, skipvendor bool
}

func (cmd *checkCommand) Name() string { return "check" }
func (cmd *checkCommand) Args() string {
	return "[-q] [-skip-lock] [-skip-vendor]"
}
func (cmd *checkCommand) ShortHelp() string { return checkShortHelp }
func (cmd *checkCommand) LongHelp() string  { return checkLongHelp }
func (cmd *checkCommand) Hidden() bool      { return false }

func (cmd *checkCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.skiplock, "skip-lock", false, "Skip checking that imports and Gopkg.toml are in sync with Gopkg.lock")
	fs.BoolVar(&cmd.skipvendor, "skip-vendor", false, "Skip checking that vendor is in sync with Gopkg.lock")
	fs.BoolVar(&cmd.quiet, "q", false, "Suppress non-error output")
}

func (cmd *checkCommand) Run(ctx *dep.Ctx, args []string) error {
	logger := ctx.Out
	if cmd.quiet {
		logger = log.New(ioutil.Discard, "", 0)
	}

	p, err := ctx.LoadProject()
	if err != nil {
		return err
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return err
	}

	sm.UseDefaultSignalHandling()
	defer sm.Release()

	var fail bool
	if !cmd.skiplock {
		if p.Lock == nil {
			return errors.New("Gopkg.lock does not exist, cannot check it against imports and Gopkg.toml")
		}

		lsat := verify.LockSatisfiesInputs(p.Lock, p.Manifest, p.RootPackageTree)
		delta := verify.DiffLocks(p.Lock, p.ChangedLock)
		sat, changed := lsat.Satisfied(), delta.Changed(verify.PruneOptsChanged|verify.HashVersionChanged)

		if changed || !sat {
			fail = true
			logger.Println("# Gopkg.lock is out of sync:")
			if !sat {
				logger.Printf("%s\n", sprintLockUnsat(lsat))
			}
			if changed {
				for pr, lpd := range delta.ProjectDeltas {
					// Only two possible changes right now are prune opts
					// changing or a missing hash digest (for old Gopkg.lock
					// files)
					if lpd.PruneOptsChanged() {
						// Override what's on the lockdiff with the extra info we have;
						// this lets us excise PruneNestedVendorDirs and get the real
						// value from the input param in place.
						old := lpd.PruneOptsBefore & ^gps.PruneNestedVendorDirs
						new := lpd.PruneOptsAfter & ^gps.PruneNestedVendorDirs
						logger.Printf("%s: prune options changed (%s -> %s)\n", pr, old, new)
					}
					if lpd.HashVersionWasZero() {
						logger.Printf("%s: no hash digest in lock\n", pr)
					}
				}
			}
		}
	}

	if !cmd.skipvendor {
		if p.Lock == nil {
			return errors.New("Gopkg.lock does not exist, cannot check vendor against it")
		}

		statuses, err := p.VerifyVendor()
		if err != nil {
			return errors.Wrap(err, "error while verifying vendor")
		}

		if fail {
			logger.Println()
		}
		// One full pass through, to see if we need to print the header.
		for _, status := range statuses {
			if status != verify.NoMismatch {
				fail = true
				logger.Println("# vendor is out of sync:")
				break
			}
		}

		for pr, status := range statuses {
			switch status {
			case verify.NotInTree:
				logger.Printf("%s: missing from vendor\n", pr)
			case verify.NotInLock:
				fi, err := os.Stat(filepath.Join(p.AbsRoot, "vendor", pr))
				if err != nil {
					return errors.Wrap(err, "could not stat file that VerifyVendor claimed existed")
				}

				if fi.IsDir() {
					logger.Printf("%s: unused project\n", pr)
				} else {
					logger.Printf("%s: orphaned file\n", pr)
				}
			case verify.DigestMismatchInLock:
				logger.Printf("%s: hash of vendored tree didn't match digest in Gopkg.lock\n", pr)
			case verify.HashVersionMismatch:
				// This will double-print if the hash version is zero, but
				// that's a rare case that really only occurs before the first
				// run with a version of dep >=0.5.0, so it's fine.
				logger.Printf("%s: hash algorithm mismatch, want version %v\n", pr, verify.HashVersion)
			}
		}
	}

	if fail {
		return silentfail{}
	}
	return nil
}

func sprintLockUnsat(lsat verify.LockSatisfaction) string {
	var buf bytes.Buffer
	for _, missing := range lsat.MissingImports {
		fmt.Fprintf(&buf, "%s: missing from input-imports\n", missing)
	}
	for _, excess := range lsat.ExcessImports {
		fmt.Fprintf(&buf, "%s: in input-imports, but not imported\n", excess)
	}
	for pr, unmatched := range lsat.UnmetOverrides {
		fmt.Fprintf(&buf, "%s@%s: not allowed by override %s\n", pr, unmatched.V, unmatched.C)
	}
	for pr, unmatched := range lsat.UnmetConstraints {
		fmt.Fprintf(&buf, "%s@%s: not allowed by constraint %s\n", pr, unmatched.V, unmatched.C)
	}
	return strings.TrimSpace(buf.String())
}
