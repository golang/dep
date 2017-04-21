// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/dep"
	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
	"github.com/sdboyer/gps/pkgtree"
)

const ensureShortHelp = `Ensure a dependency is safely vendored in the project`
const ensureLongHelp = `
Ensure is used to fetch project dependencies into the vendor folder, as well as
to set version constraints for specific dependencies. It takes user input,
solves the updated dependency graph of the project, writes any changes to the
manifest and lock file, and places dependencies in the vendor folder.

Package spec:

  <path>[:alt location][@<version specifier>]

Examples:

  dep ensure                            Populate vendor from existing manifest and lock
  dep ensure github.com/pkg/foo@^1.0.1  Update a specific dependency to a specific version

For more detailed usage examples, see dep ensure -examples.
`
const ensureExamples = `
dep ensure

    Solve the project's dependency graph, and place all dependencies in the
    vendor folder. If a dependency is in the lock file, use the version
    specified there. Otherwise, use the most recent version that can satisfy the
    constraints in the manifest file.

dep ensure -update

    Update all dependencies to the latest versions allowed by the manifest,
    ignoring any versions specified in the lock file. Update the lock file with
    any changes.

dep ensure -update github.com/pkg/foo github.com/pkg/bar

    Update a list of dependencies to the latest versions allowed by the manifest,
    ignoring any versions specified in the lock file. Update the lock file with
    any changes.

dep ensure github.com/pkg/foo@^1.0.1

    Constrain pkg/foo to the latest release matching >= 1.0.1, < 2.0.0, and
    place that release in the vendor folder. If a constraint was previously set
    in the manifest, this resets it. This form of constraint strikes a good
    balance of safety and flexibility, and should be preferred for libraries.

dep ensure github.com/pkg/foo@~1.0.1

    Same as above, but choose any release matching 1.0.x, preferring latest.

dep ensure github.com/pkg/foo:git.internal.com/alt/foo

    Fetch the dependency from a different location.

dep ensure -override github.com/pkg/foo@^1.0.1

    Forcefully and transitively override any constraint for this dependency.
    Overrides are powerful, but harmful in the long term. They should be used as
    a last resort, especially if your project may be imported by others.
`

func (cmd *ensureCommand) Name() string      { return "ensure" }
func (cmd *ensureCommand) Args() string      { return "[spec...]" }
func (cmd *ensureCommand) ShortHelp() string { return ensureShortHelp }
func (cmd *ensureCommand) LongHelp() string  { return ensureLongHelp }
func (cmd *ensureCommand) Hidden() bool      { return false }

func (cmd *ensureCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.examples, "examples", false, "print detailed usage examples")
	fs.BoolVar(&cmd.update, "update", false, "ensure dependencies are at the latest version allowed by the manifest")
	fs.BoolVar(&cmd.dryRun, "n", false, "dry run, don't actually ensure anything")
	fs.Var(&cmd.overrides, "override", "specify an override constraint spec (repeatable)")
}

type ensureCommand struct {
	examples  bool
	update    bool
	dryRun    bool
	overrides stringSlice
}

func (cmd *ensureCommand) Run(ctx *dep.Ctx, args []string) error {
	if cmd.examples {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(ensureExamples))
		return nil
	}

	p, err := ctx.LoadProject("")
	if err != nil {
		return err
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return err
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	params := p.MakeParams()
	if *verbose {
		params.Trace = true
		params.TraceLogger = log.New(os.Stderr, "", 0)
	}
	params.RootPackageTree, err = pkgtree.ListPackages(p.AbsRoot, string(p.ImportRoot))
	if err != nil {
		return errors.Wrap(err, "ensure ListPackage for project")
	}

	if cmd.update {
		applyUpdateArgs(args, &params)
	} else {
		err := applyEnsureArgs(args, cmd.overrides, p, sm, &params)
		if err != nil {
			return err
		}
	}

	solver, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "ensure Prepare")
	}
	solution, err := solver.Solve()
	if err != nil {
		handleAllTheFailuresOfTheWorld(err)
		return errors.Wrap(err, "ensure Solve()")
	}

	// check if vendor exists, because if the locks are the same but
	// vendor does not exist we should write vendor
	vendorExists, err := dep.IsNonEmptyDir(filepath.Join(p.AbsRoot, "vendor"))
	if err != nil {
		return errors.Wrap(err, "ensure vendor is a directory")
	}
	writeV := dep.VendorOnChanged
	if !vendorExists && solution != nil {
		writeV = dep.VendorAlways
	}

	var sw dep.SafeWriter
	var manifest *dep.Manifest
	if !cmd.update {
		manifest = p.Manifest
	}

	newLock := dep.LockFromInterface(solution)
	sw.Prepare(manifest, p.Lock, newLock, writeV)
	if cmd.dryRun {
		return sw.PrintPreparedActions()
	}

	return errors.Wrap(sw.Write(p.AbsRoot, sm), "grouped write of manifest, lock and vendor")
}

func applyUpdateArgs(args []string, params *gps.SolveParameters) {
	// When -update is specified without args, allow every project to change versions, regardless of the lock file
	if len(args) == 0 {
		params.ChangeAll = true
		return
	}

	// Allow any of specified project versions to change, regardless of the lock file
	for _, arg := range args {
		params.ToChange = append(params.ToChange, gps.ProjectRoot(arg))
	}
}

func applyEnsureArgs(args []string, overrides stringSlice, p *dep.Project, sm *gps.SourceMgr, params *gps.SolveParameters) error {
	var errs []error
	for _, arg := range args {
		pc, err := getProjectConstraint(arg, sm)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if gps.IsAny(pc.Constraint) && pc.Ident.Source == "" {
			// If the input specified neither a network name nor a constraint,
			// then the strict thing to do would be to remove the entry
			// entirely. But that would probably be quite surprising for users,
			// and it's what rm is for, so just ignore the input.
			//
			// TODO(sdboyer): for this case - or just in general - do we want to
			// add project args to the requires list temporarily for this run?
			if _, has := p.Manifest.Dependencies[pc.Ident.ProjectRoot]; !has {
				logf("No constraint or alternate source specified for %q, omitting from manifest", pc.Ident.ProjectRoot)
			}
			// If it's already in the manifest, no need to log
			continue
		}

		p.Manifest.Dependencies[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			Source:     pc.Ident.Source,
			Constraint: pc.Constraint,
		}
	}

	for _, ovr := range overrides {
		pc, err := getProjectConstraint(ovr, sm)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Empty overrides are fine (in contrast to deps), because they actually
		// carry meaning - they force the constraints entirely open for a given
		// project. Inadvisable, but meaningful.

		p.Manifest.Ovr[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			Source:     pc.Ident.Source,
			Constraint: pc.Constraint,
		}
	}

	if len(errs) > 0 {
		var buf bytes.Buffer
		for _, err := range errs {
			fmt.Fprintln(&buf, err)
		}

		return errors.New(buf.String())
	}

	return nil
}

type stringSlice []string

func (s *stringSlice) String() string {
	if len(*s) == 0 {
		return "<none>"
	}
	return strings.Join(*s, ", ")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func getProjectConstraint(arg string, sm *gps.SourceMgr) (gps.ProjectConstraint, error) {
	constraint := gps.ProjectConstraint{
		Constraint: gps.Any(), // default to any; avoids panics later
	}

	// try to split on '@'
	var versionStr string
	atIndex := strings.Index(arg, "@")
	if atIndex > 0 {
		parts := strings.SplitN(arg, "@", 2)
		arg = parts[0]
		versionStr = parts[1]
		constraint.Constraint = deduceConstraint(parts[1])
	}
	// TODO: What if there is no @, assume default branch (which may not be master) ?
	// TODO: if we decide to keep equals.....

	// split on colon if there is a network location
	colonIndex := strings.Index(arg, ":")
	if colonIndex > 0 {
		parts := strings.SplitN(arg, ":", 2)
		arg = parts[0]
		constraint.Ident.Source = parts[1]
	}

	pr, err := sm.DeduceProjectRoot(arg)
	if err != nil {
		return constraint, errors.Wrapf(err, "could not infer project root from dependency path: %s", arg) // this should go through to the user
	}

	if string(pr) != arg {
		return constraint, errors.Errorf("dependency path %s is not a project root, try %s instead", arg, pr)
	}

	constraint.Ident.ProjectRoot = gps.ProjectRoot(arg)

	// Below we are checking if the constraint we deduced was valid.
	if v, ok := constraint.Constraint.(gps.Version); ok && versionStr != "" {
		if v.Type() == gps.IsVersion {
			// we hit the fall through case in deduce constraint, let's call out to network
			// and get the package's versions
			versions, err := sm.ListVersions(constraint.Ident)
			if err != nil {
				return constraint, errors.Wrapf(err, "list versions for %s", arg) // means repo does not exist
			}

			var found bool
			for _, version := range versions {
				if versionStr == version.String() {
					found = true
					constraint.Constraint = version.Unpair()
					break
				}
			}

			if !found {
				return constraint, errors.Errorf("%s is not a valid version for the package %s", versionStr, arg)
			}
		}
	}

	return constraint, nil
}

// deduceConstraint tries to puzzle out what kind of version is given in a string -
// semver, a revision, or as a fallback, a plain tag
func deduceConstraint(s string) gps.Constraint {
	// always semver if we can
	c, err := gps.NewSemverConstraint(s)
	if err == nil {
		return c
	}

	slen := len(s)
	if slen == 40 {
		if _, err = hex.DecodeString(s); err == nil {
			// Whether or not it's intended to be a SHA1 digest, this is a
			// valid byte sequence for that, so go with Revision. This
			// covers git and hg
			return gps.Revision(s)
		}
	}
	// Next, try for bzr, which has a three-component GUID separated by
	// dashes. There should be two, but the email part could contain
	// internal dashes
	if strings.Count(s, "-") >= 2 {
		// Work from the back to avoid potential confusion from the email
		i3 := strings.LastIndex(s, "-")
		// Skip if - is last char, otherwise this would panic on bounds err
		if slen == i3+1 {
			return gps.NewVersion(s)
		}

		i2 := strings.LastIndex(s[:i3], "-")
		if _, err = strconv.ParseUint(s[i2+1:i3], 10, 64); err == nil {
			// Getting this far means it'd pretty much be nuts if it's not a
			// bzr rev, so don't bother parsing the email.
			return gps.Revision(s)
		}
	}

	// If not a plain SHA1 or bzr custom GUID, assume a plain version.
	// TODO: if there is amgibuity here, then prompt the user?
	return gps.NewVersion(s)
}
