// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/pkgtree"
	"github.com/golang/dep/internal"
	"github.com/pkg/errors"
)

const ensureShortHelp = `Ensure a dependency is safely vendored in the project`
const ensureLongHelp = `
usage: dep ensure [-update | -add] [-no-vendor | -vendor-only] [<spec>...]

Project spec:

  <path>[:alt source][@<constraint>]

Flags:

  -update: update all, or only the named, dependencies in Gopkg.lock
  -add: add new dependencies
  -no-vendor: update Gopkg.lock if needed, but do not update vendor/
  -vendor-only: populate vendor/ without updating Gopkg.lock


Ensure gets a project into a complete, compilable, and reproducible state:

  * All non-stdlib imports are fulfilled
  * All constraints and overrides in Gopkg.toml are respected
  * Gopkg.lock records precise versions for all dependencies
  * vendor/ is populated according to Gopkg.lock

Ensure has fast techniques to determine that some of these steps may be
unnecessary. If that determination is made, ensure may skip some steps. Flags
may be passed to bypass these checks; -vendor-only will allow an out-of-date
Gopkg.lock to populate vendor/, and -no-vendor will update Gopkg.lock (if
needed), but never touch vendor/.

The effect of passing project spec arguments varies slightly depending on the
combination of flags that are passed.


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
	fs.BoolVar(&cmd.update, "update", false, "update the named dependencies (or all, if none are named) in Gopkg.lock to the latest allowed by Gopkg.toml")
	fs.BoolVar(&cmd.add, "add", false, "add new dependencies, or populate Gopkg.toml with constraints for existing dependencies")
	fs.BoolVar(&cmd.vendorOnly, "vendor-only", false, "populate vendor/ from Gopkg.lock without updating it first")
	fs.BoolVar(&cmd.noVendor, "no-vendor", false, "update Gopkg.lock (if needed), but do not update vendor/")
}

type ensureCommand struct {
	examples   bool
	update     bool
	add        bool
	noVendor   bool
	vendorOnly bool
	overrides  stringSlice
}

func (cmd *ensureCommand) Run(ctx *dep.Ctx, args []string) error {
	if cmd.examples {
		internal.Logln(strings.TrimSpace(ensureExamples))
		return nil
	}

	if cmd.add && cmd.update {
		return errors.New("cannot pass both -add and -update")
	}

	if cmd.vendorOnly {
		if cmd.update {
			return errors.New("-vendor-only makes -update a no-op; cannot pass them together")
		}
		if cmd.add {
			return errors.New("-vendor-only makes -add a no-op; cannot pass them together")
		}
		if cmd.noVendor {
			// TODO(sdboyer) can't think of anything not snarky right now
			return errors.New("really?")
		}
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
		params.TraceLogger = log.New(os.Stderr, "", 0)
	}
	params.RootPackageTree, err = pkgtree.ListPackages(p.AbsRoot, string(p.ImportRoot))
	if err != nil {
		return errors.Wrap(err, "ensure ListPackage for project")
	}

	if err := checkErrors(params.RootPackageTree.Packages); err != nil {
		return err
	}

	var fail error
	if cmd.add {
		return cmd.runAdd(ctx, args, p, sm, params)
	} else if cmd.update {
		return cmd.runUpdate(ctx, args, p, sm, params)
	}
	return cmd.runDefault(ctx, args, p, sm, params)

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

	newLock := dep.LockFromInterface(solution)
	sw, err := dep.NewSafeWriter(nil, p.Lock, newLock, writeV)
	if err != nil {
		return err
	}
	if cmd.dryRun {
		return sw.PrintPreparedActions()
	}

	return errors.Wrap(sw.Write(p.AbsRoot, sm, true), "grouped write of manifest, lock and vendor")
}

func (cmd *ensureCommand) runDefault(ctx *dep.Ctx, args []string, p *dep.Project, sm gps.SourceManager, params gps.SolveParameters) error {
	// Bare ensure doesn't take any args.
	if len(args) != 0 {
		if cmd.vendorOnly {
			return errors.Errorf("dep ensure -vendor-only only populates vendor/ from %s; it takes no spec arguments.", dep.LockName)
		}
		return errors.New("dep ensure only takes spec arguments with -add or -update - did you want one of those?")
	}

	sw := &dep.SafeWriter{}
	if cmd.vendorOnly {
		if p.Lock == nil {
			return errors.Errorf("no %s exists from which to populate vendor/ directory", dep.LockName)
		}
		// Pass the same lock as old and new so that the writer will observe no
		// difference and choose not to write it out.
		err := sw.Prepare(nil, p.Lock, p.Lock, dep.VendorAlways)
		if err != nil {
			return err
		}

		if cmd.dryRun {
			fmt.Printf("Would have populated vendor/ directory from %s", dep.LockName)
			return nil
		}

		return errors.WithMessage(sw.Write(p.AbsRoot, sm, true), "grouped write of manifest, lock and vendor")
	}

	solver, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "prepare solver")
	}

	if p.Lock != nil && bytes.Equal(p.Lock.InputHash(), solver.HashInputs()) {
		// Memo matches, so there's probably nothing to do.
		if cmd.noVendor {
			// The user said not to touch vendor/, so definitely nothing to do.
			return nil
		}

		// TODO(sdboyer) The desired behavior at this point is to determine
		// whether it's necessary to write out vendor, or if it's already
		// consistent with the lock. However, we haven't yet determined what
		// that "verification" is supposed to look like (#121); in the meantime,
		// we unconditionally write out vendor/ so that `dep ensure`'s behavior
		// is maximally compatible with what it will eventually become.
		err := sw.Prepare(nil, p.Lock, p.Lock, dep.VendorAlways)
		if err != nil {
			return err
		}

		if cmd.dryRun {
			fmt.Printf("Would have populated vendor/ directory from %s", dep.LockName)
			return nil
		}

		return errors.WithMessage(sw.Write(p.AbsRoot, sm, true), "grouped write of manifest, lock and vendor")
	}

	solution, err := solver.Solve()
	if err != nil {
		handleAllTheFailuresOfTheWorld(err)
		return errors.Wrap(err, "ensure Solve()")
	}

	sw.Prepare(nil, p.Lock, dep.LockFromInterface(solution), dep.VendorOnChanged)
	if cmd.dryRun {
		return sw.PrintPreparedActions()
	}

	return nil
}

func (cmd *ensureCommand) runUpdate(ctx *dep.Ctx, args []string, p *dep.Project, sm gps.SourceManager, params gps.SolveParameters) error {
	if p.Lock == nil {
		return errors.New("%s does not exist. nothing to do, as -update works by updating the values in %s.", dep.LockName, dep.LockName)
	}

	// We'll need to discard this prepared solver as later work changes params,
	// but solver preparation is cheap and worth doing up front in order to
	// perform the fastpath check of hash comparison.
	solver, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "fastpath solver prepare")
	}

	// Compare the hashes. If they're not equal, bail out and ask the user to
	// run a straight `dep ensure` before updating. This is handholding the
	// user a bit, but the extra effort required is minimal, and it ensures the
	// user is isolating variables in the event of solve problems (was it the
	// "pending" changes, or the -update that caused the problem?).
	// TODO(sdboyer) reduce this to a warning?
	if bytes.Equal(p.Lock.InputHash(), solver.HashInputs()) {
		return errors.Errorf("%s and %s are out of sync. run a plain `dep ensure` to resync them before attempting an -update.", dep.ManifestName, dep.LockName)
	}

	// When -update is specified without args, allow every dependency to change
	// versions, regardless of the lock file.
	if len(args) == 0 {
		params.ChangeAll = true
		return
	}

	// Allow any of specified project versions to change, regardless of the lock
	// file.
	for _, arg := range args {
		// Ensure the provided path has a deducible project root
		// TODO(sdboyer) do these concurrently
		pc, err := getProjectConstraint(arg, sm)
		if err != nil {
			// TODO(sdboyer) return all errors, not just the first one we encounter
			// TODO(sdboyer) ensure these errors are contextualized in a sensible way for -update
			return err
		}

		if !p.Lock.HasProjectWithRoot(pc.Ident.ProjectRoot) {
			return errors.Errorf("%s is not present in %s, cannot -update it", pc.Ident.ProjectRoot, dep.LockName)
		}

		if p.Ident.Source != "" {
			return errors.Errorf("cannot specify alternate sources on -update (%s)")
		}

		if !gps.IsAny(pc.Constraint) {
			// TODO(sdboyer) constraints should be allowed to allow solves that
			// target particular versions while remaining within declared constraints
			return errors.Errorf("-update operates according to constraints declared in %s, not CLI arguments.\nYou passed in %s for %s", dep.ManifestName, pc.Constraint, pc.Ident.ProjectRoot)
		}

		params.ToChange = append(params.ToChange, gps.ProjectRoot(arg))
	}

	// Re-prepare a solver now that our params are complete.
	solver, err = gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "fastpath solver prepare")
	}
	solution, err := solver.Solve()
	if err != nil {
		handleAllTheFailuresOfTheWorld(err)
		return errors.Wrap(err, "ensure Solve()")
	}

	var sw dep.SafeWriter
	sw.Prepare(nil, p.Lock, dep.LockFromInterface(solution), dep.VendorOnChanged)
	// TODO(sdboyer) special handling for warning cases as described in spec -
	// e.g., named projects did not upgrade even though newer versions were
	// available.
	if cmd.dryRun {
		return sw.PrintPreparedActions()
	}

	return errors.Wrap(sw.Write(p.AbsRoot, sm, true), "grouped write of manifest, lock and vendor")
}

func (cmd *ensureCommand) runAdd(ctx *dep.Ctx, args []string, p *dep.Project, sm gps.SourceManager, params gps.SolveParameters) error {
	if len(args) == 0 {
		return errors.New("must specify at least one project or package to add")
	}

	// Compare the hashes. If they're not equal, bail out and ask the user to
	// run a straight `dep ensure` before updating. This is handholding the
	// user a bit, but the extra effort required is minimal, and it ensures the
	// user is isolating variables in the event of solve problems (was it the
	// "pending" changes, or the -add that caused the problem?).
	// TODO(sdboyer) reduce this to a warning?
	if bytes.Equal(p.Lock.InputHash(), solver.HashInputs()) {
		return errors.Errorf("%s and %s are out of sync. run a plain `dep ensure` to resync them before attempting an -add.", dep.ManifestName, dep.LockName)
	}

	rm, errmap := params.RootPackageTree.ToReachMap(true, true, false, p.Manifest.IgnoredPackages())
	// Having some problematic internal packages isn't cause for termination,
	// but the user needs to be warned.
	for fail := range errmap {
		internal.Logf("Warning: %s", fail)
	}

	exmap := make(map[string]bool)
	exrmap := make(map[gps.ProjectRoot]bool)
	for _, ex := range append(rm.Flatten(false), p.Manifest.RequiredPackages()...) {
		exmap[ex] = true
		root, err := sm.DeduceProjectRoot(ex)
		if err != nil {
			// This should be essentially impossible to hit, as it entails that
			// we couldn't deduce the root for an import, but that some previous
			// solve run WAS able to deduce the root.
			return errors.Wrap(err, "could not deduce project root")
		}
		exrmap[root] = true
	}

	var reqlist []string
	//pclist := make(map[gps.ProjectRoot]gps.ProjectConstraint)

	for _, arg := range args {
		// TODO(sdboyer) return all errors, not just the first one we encounter
		// TODO(sdboyer) do these concurrently
		pc, err := getProjectConstraint(arg, sm)
		if err != nil {
			// TODO(sdboyer) ensure these errors are contextualized in a sensible way for -add
			return err
		}

		inManifest := p.Manifest.HasConstraintsOn(pc.Ident.ProjectRoot)
		inImports := exrmap[pc.Ident.ProjectRoot]
		if inManifest && inImports {
			return errors.Errorf("%s is already in %s and the project's direct imports or required list; nothing to add", pc.Ident.ProjectRoot, dep.ManifestName)
		}

		err = sm.SyncSourceFor(pc.Ident)
		if err != nil {
			return errors.Wrap(err, "failed to fetch source for %s", pc.Ident.ProjectRoot)
		}

		someConstraint = pc.Constraint != nil || pc.Ident.Source != ""
		if inManifest {
			if someConstraint {
				return errors.Errorf("%s already contains constraints for %s, cannot specify a version constraint or alternate source", arg, dep.ManifestName)
			}

			reqlist = append(reqlist, arg)
			p.Manifest.Required = append(p.Manifest.Required, arg)
		} else if inImports {
			if !someConstraint {
				if exmap[arg] {
					return errors.Errorf("%s is already imported or required; -add must specify a constraint, but none were provided", arg)
				}

				// TODO(sdboyer) this case seems like it's getting overly
				// specific and risks muddying the water more than it helps
				// No constraints, but the package isn't imported; require it.
				reqlist = append(reqlist, arg)
				p.Manifest.Required = append(p.Manifest.Required, arg)
			} else {
				p.Manifest.Dependencies[pc.Ident.ProjectRoot] = gps.ProjectProperties{
					Source:     pc.Ident.Source,
					Constraint: pc.Constraint,
				}

				// Don't require on this branch if the arg was a ProjectRoot;
				// most common here will be the user adding constraints to
				// something they already imported, and if they specify the
				// root, there's a good chance they don't actually want to
				// require the project's root package, but are just trying to
				// indicate which project should receive the constraints.
				if !exmap[arg] && string(pc.Ident.ProjectRoot) != arg {
					reqlist = append(reqlist, arg)
					p.Manifest.Required = append(p.Manifest.Required, arg)
				}
			}
		} else {
			p.Manifest.Dependencies[pc.Ident.ProjectRoot] = gps.ProjectProperties{
				Source:     pc.Ident.Source,
				Constraint: pc.Constraint,
			}

			reqlist = append(reqlist, arg)
			p.Manifest.Required = append(p.Manifest.Required, arg)
		}
	}

	// Re-prepare a solver now that our params are complete.
	solver, err = gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "fastpath solver prepare")
	}
	solution, err := solver.Solve()
	if err != nil {
		// TODO(sdboyer) detect if the failure was specifically about some of
		// the -add arguments
		handleAllTheFailuresOfTheWorld(err)
		return errors.Wrap(err, "ensure Solve()")
	}

	var sw dep.SafeWriter
	sw.Prepare(nil, p.Lock, dep.LockFromInterface(solution), dep.VendorOnChanged)
	// TODO(sdboyer) special handling for warning cases as described in spec -
	// e.g., named projects did not upgrade even though newer versions were
	// available.
	if cmd.dryRun {
		return sw.PrintPreparedActions()
	}

	return errors.Wrap(sw.Write(p.AbsRoot, sm, true), "grouped write of manifest, lock and vendor")
}

func applyEnsureArgs(args []string, overrides stringSlice, p *dep.Project, sm gps.SourceManager, params *gps.SolveParameters) error {
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
				internal.Logf("No constraint or alternate source specified for %q, omitting from manifest", pc.Ident.ProjectRoot)
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

func getProjectConstraint(arg string, sm gps.SourceManager) (gps.ProjectConstraint, error) {
	// TODO(sdboyer) this func needs to be broken out, now that we admit
	// different info in specs
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

func checkErrors(m map[string]pkgtree.PackageOrErr) error {
	noGoErrors, pkgErrors := 0, 0
	for _, poe := range m {
		if poe.Err != nil {
			switch poe.Err.(type) {
			case *build.NoGoError:
				noGoErrors++
			default:
				pkgErrors++
			}
		}
	}
	if len(m) == 0 || len(m) == noGoErrors {
		return errors.New("all dirs lacked any go code")
	}

	if len(m) == pkgErrors {
		return errors.New("all dirs had go code with errors")
	}

	if len(m) == pkgErrors+noGoErrors {
		return errors.Errorf("%d dirs had errors and %d had no go code", pkgErrors, noGoErrors)
	}

	return nil
}
