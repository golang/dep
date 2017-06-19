// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"go/build"
	"strconv"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/paths"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pkg/errors"
)

const ensureShortHelp = `Ensure a dependency is safely vendored in the project`
const ensureLongHelp = `
Project spec:

  <path>[:alt source][@<constraint>]

Flags:

  -update: update all, or only the named, dependencies in Gopkg.lock
  -add: add new dependencies
  -no-vendor: update Gopkg.lock if needed, but do not update vendor/
  -vendor-only: populate vendor/ without updating Gopkg.lock
  -dry-run: only report the changes that would be made


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

func (cmd *ensureCommand) Name() string { return "ensure" }
func (cmd *ensureCommand) Args() string {
	return "[-update | -add] [-no-vendor | -vendor-only] [<spec>...]"
}
func (cmd *ensureCommand) ShortHelp() string { return ensureShortHelp }
func (cmd *ensureCommand) LongHelp() string  { return ensureLongHelp }
func (cmd *ensureCommand) Hidden() bool      { return false }

func (cmd *ensureCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.examples, "examples", false, "print detailed usage examples")
	fs.BoolVar(&cmd.update, "update", false, "update the named dependencies (or all, if none are named) in Gopkg.lock to the latest allowed by Gopkg.toml")
	fs.BoolVar(&cmd.add, "add", false, "add new dependencies, or populate Gopkg.toml with constraints for existing dependencies")
	fs.BoolVar(&cmd.vendorOnly, "vendor-only", false, "populate vendor/ from Gopkg.lock without updating it first")
	fs.BoolVar(&cmd.noVendor, "no-vendor", false, "update Gopkg.lock (if needed), but do not update vendor/")
	fs.BoolVar(&cmd.dryRun, "dry-run", false, "only report the changes that would be made")
}

type ensureCommand struct {
	examples   bool
	update     bool
	add        bool
	noVendor   bool
	vendorOnly bool
	dryRun     bool
	overrides  stringSlice
}

func (cmd *ensureCommand) Run(ctx *dep.Ctx, args []string) error {
	if cmd.examples {
		ctx.Err.Println(strings.TrimSpace(ensureExamples))
		return nil
	}

	if err := cmd.validateFlags(); err != nil {
		return err
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

	params := p.MakeParams()
	if ctx.Verbose {
		params.TraceLogger = ctx.Err
	}

	params.RootPackageTree, err = pkgtree.ListPackages(p.ResolvedAbsRoot, string(p.ImportRoot))
	if err != nil {
		return errors.Wrap(err, "ensure ListPackage for project")
	}

	if err := checkErrors(params.RootPackageTree.Packages); err != nil {
		return err
	}

	if cmd.add {
		return cmd.runAdd(ctx, args, p, sm, params)
	} else if cmd.update {
		return cmd.runUpdate(ctx, args, p, sm, params)
	}
	return cmd.runDefault(ctx, args, p, sm, params)
}

func (cmd *ensureCommand) validateFlags() error {
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
	return nil
}

func (cmd *ensureCommand) runDefault(ctx *dep.Ctx, args []string, p *dep.Project, sm gps.SourceManager, params gps.SolveParameters) error {
	// Bare ensure doesn't take any args.
	if len(args) != 0 {
		if cmd.vendorOnly {
			return errors.Errorf("dep ensure -vendor-only only populates vendor/ from %s; it takes no spec arguments", dep.LockName)
		}
		return errors.New("dep ensure only takes spec arguments with -add or -update")
	}

	if cmd.vendorOnly {
		if p.Lock == nil {
			return errors.Errorf("no %s exists from which to populate vendor/", dep.LockName)
		}
		// Pass the same lock as old and new so that the writer will observe no
		// difference and choose not to write it out.
		sw, err := dep.NewSafeWriter(nil, p.Lock, p.Lock, dep.VendorAlways)
		if err != nil {
			return err
		}

		if cmd.dryRun {
			ctx.Out.Printf("Would have populated vendor/ directory from %s", dep.LockName)
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
		sw, err := dep.NewSafeWriter(nil, p.Lock, p.Lock, dep.VendorAlways)
		if err != nil {
			return err
		}

		if cmd.dryRun {
			ctx.Out.Printf("Would have populated vendor/ directory from %s", dep.LockName)
			return nil
		}

		return errors.WithMessage(sw.Write(p.AbsRoot, sm, true), "grouped write of manifest, lock and vendor")
	}

	solution, err := solver.Solve()
	if err != nil {
		handleAllTheFailuresOfTheWorld(err)
		return errors.Wrap(err, "ensure Solve()")
	}

	sw, err := dep.NewSafeWriter(nil, p.Lock, dep.LockFromSolution(solution), dep.VendorOnChanged)
	if err != nil {
		return err
	}
	if cmd.dryRun {
		return sw.PrintPreparedActions(ctx.Out)
	}

	return errors.Wrap(sw.Write(p.AbsRoot, sm, false), "grouped write of manifest, lock and vendor")
}

func (cmd *ensureCommand) runUpdate(ctx *dep.Ctx, args []string, p *dep.Project, sm gps.SourceManager, params gps.SolveParameters) error {
	if p.Lock == nil {
		return errors.Errorf("-update works by updating the versions recorded in %s, but %s does not exist", dep.LockName, dep.LockName)
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
	if !bytes.Equal(p.Lock.InputHash(), solver.HashInputs()) {
		return errors.Errorf("%s and %s are out of sync. Run a plain dep ensure to resync them before attempting to -update", dep.ManifestName, dep.LockName)
	}

	// When -update is specified without args, allow every dependency to change
	// versions, regardless of the lock file.
	if len(args) == 0 {
		params.ChangeAll = true
	}

	// Allow any of specified project versions to change, regardless of the lock
	// file.
	for _, arg := range args {
		// Ensure the provided path has a deducible project root
		// TODO(sdboyer) do these concurrently
		pc, path, err := getProjectConstraint(arg, sm)
		if err != nil {
			// TODO(sdboyer) return all errors, not just the first one we encounter
			// TODO(sdboyer) ensure these errors are contextualized in a sensible way for -update
			return err
		}
		if path != string(pc.Ident.ProjectRoot) {
			// TODO(sdboyer): does this really merit an abortive error?
			return errors.Errorf("%s is not a project root, try %s instead", path, pc.Ident.ProjectRoot)
		}

		if !p.Lock.HasProjectWithRoot(pc.Ident.ProjectRoot) {
			return errors.Errorf("%s is not present in %s, cannot -update it", pc.Ident.ProjectRoot, dep.LockName)
		}

		if pc.Ident.Source != "" {
			return errors.Errorf("cannot specify alternate sources on -update (%s)", pc.Ident.Source)
		}

		if !gps.IsAny(pc.Constraint) {
			// TODO(sdboyer) constraints should be allowed to allow solves that
			// target particular versions while remaining within declared constraints
			return errors.Errorf("version constraint %s passed for %s, but -update follows constraints declared in %s, not CLI arguments", pc.Constraint, pc.Ident.ProjectRoot, dep.ManifestName)
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

	sw, err := dep.NewSafeWriter(nil, p.Lock, dep.LockFromSolution(solution), dep.VendorOnChanged)
	if err != nil {
		return err
	}
	// TODO(sdboyer) special handling for warning cases as described in spec -
	// e.g., named projects did not upgrade even though newer versions were
	// available.
	if cmd.dryRun {
		return sw.PrintPreparedActions(ctx.Out)
	}

	return errors.Wrap(sw.Write(p.AbsRoot, sm, false), "grouped write of manifest, lock and vendor")
}

func (cmd *ensureCommand) runAdd(ctx *dep.Ctx, args []string, p *dep.Project, sm gps.SourceManager, params gps.SolveParameters) error {
	if len(args) == 0 {
		return errors.New("must specify at least one project or package to -add")
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
	// "pending" changes, or the -add that caused the problem?).
	// TODO(sdboyer) reduce this to a warning?
	if p.Lock != nil && !bytes.Equal(p.Lock.InputHash(), solver.HashInputs()) {
		return errors.Errorf("%s and %s are out of sync. Run a plain dep ensure to resync them before attempting to -add", dep.ManifestName, dep.LockName)
	}

	rm, errmap := params.RootPackageTree.ToReachMap(true, true, false, p.Manifest.IgnoredPackages())
	// Having some problematic internal packages isn't cause for termination,
	// but the user needs to be warned.
	for fail := range errmap {
		ctx.Err.Printf("Warning: %s", fail)
	}

	// Compile unique sets of 1) all external packages imported or required, and
	// 2) the project roots under which they fall.
	exmap := make(map[string]bool)
	exrmap := make(map[gps.ProjectRoot]bool)

	for _, ex := range append(rm.FlattenFn(paths.IsStandardImportPath), p.Manifest.Required...) {
		exmap[ex] = true
		root, err := sm.DeduceProjectRoot(ex)
		if err != nil {
			// This should be very uncommon to hit, as it entails that we
			// couldn't deduce the root for an import, but that some previous
			// solve run WAS able to deduce the root. It's most likely to occur
			// if the user has e.g. not connected to their organization's VPN,
			// and thus cannot access an internal go-get metadata service.
			return errors.Wrapf(err, "could not deduce project root for %s", ex)
		}
		exrmap[root] = true
	}

	var reqlist []string

	for _, arg := range args {
		// TODO(sdboyer) return all errors, not just the first one we encounter
		// TODO(sdboyer) do these concurrently
		pc, path, err := getProjectConstraint(arg, sm)
		if err != nil {
			// TODO(sdboyer) ensure these errors are contextualized in a sensible way for -add
			return err
		}

		inManifest := p.Manifest.HasConstraintsOn(pc.Ident.ProjectRoot)
		inImports := exrmap[pc.Ident.ProjectRoot]
		if inManifest && inImports {
			return errors.Errorf("nothing to -add, %s is already in %s and the project's direct imports or required list", pc.Ident.ProjectRoot, dep.ManifestName)
		}

		err = sm.SyncSourceFor(pc.Ident)
		if err != nil {
			return errors.Wrapf(err, "failed to fetch source for %s", pc.Ident.ProjectRoot)
		}

		someConstraint := !gps.IsAny(pc.Constraint) || pc.Ident.Source != ""
		if inManifest {
			if someConstraint {
				return errors.Errorf("%s already contains rules for %s, cannot specify a version constraint or alternate source", dep.ManifestName, path)
			}

			reqlist = append(reqlist, path)
			p.Manifest.Required = append(p.Manifest.Required, path)
		} else if inImports {
			if !someConstraint {
				if exmap[path] {
					return errors.Errorf("%s is already imported or required; -add must specify a constraint, but none were provided", path)
				}

				// No constraints, but the package isn't imported; require it.
				// TODO(sdboyer) this case seems like it's getting overly
				// specific and risks muddying the water more than it helps
				reqlist = append(reqlist, path)
				p.Manifest.Required = append(p.Manifest.Required, path)
			} else {
				p.Manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{
					Source:     pc.Ident.Source,
					Constraint: pc.Constraint,
				}

				// Don't require on this branch if the path was a ProjectRoot;
				// most common here will be the user adding constraints to
				// something they already imported, and if they specify the
				// root, there's a good chance they don't actually want to
				// require the project's root package, but are just trying to
				// indicate which project should receive the constraints.
				if !exmap[path] && string(pc.Ident.ProjectRoot) != path {
					reqlist = append(reqlist, path)
					p.Manifest.Required = append(p.Manifest.Required, path)
				}
			}
		} else {
			p.Manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{
				Source:     pc.Ident.Source,
				Constraint: pc.Constraint,
			}

			reqlist = append(reqlist, path)
			p.Manifest.Required = append(p.Manifest.Required, path)
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

	sw, err := dep.NewSafeWriter(nil, p.Lock, dep.LockFromSolution(solution), dep.VendorOnChanged)
	// TODO(sdboyer) special handling for warning cases as described in spec -
	// e.g., named projects did not upgrade even though newer versions were
	// available.
	if cmd.dryRun {
		return sw.PrintPreparedActions(ctx.Out)
	}

	err = errors.Wrap(sw.Write(p.AbsRoot, sm, true), "grouped write of manifest, lock and vendor")
	if err != nil {
		return err
	}

	// TODO(sdboyer) handle appending of constraints to Gopkg.toml here, plus
	// info messages to user
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

func getProjectConstraint(arg string, sm gps.SourceManager) (gps.ProjectConstraint, string, error) {
	emptyPC := gps.ProjectConstraint{
		Constraint: gps.Any(), // default to any; avoids panics later
	}

	path, source, versionStr := parseSpecArg(arg)
	pr, err := sm.DeduceProjectRoot(path)
	if err != nil {
		return emptyPC, "", errors.Wrapf(err, "could not infer project root from dependency path: %s", path) // this should go through to the user
	}

	pi := gps.ProjectIdentifier{ProjectRoot: pr, Source: source}
	var c gps.Constraint
	if versionStr != "" {
		c, err = deduceConstraint(versionStr, pi, sm)
		if err != nil {
			return emptyPC, "", err
		}
	} else {
		c = gps.Any()
	}
	return gps.ProjectConstraint{Ident: pi, Constraint: c}, path, nil
}

// parseSpecArg takes a spec, as provided to dep ensure on the CLI, and splits
// it into its constitutent path, source, and constraint parts.
func parseSpecArg(arg string) (path, source, constraint string) {
	path = arg
	// try to split on '@'
	// When there is no `@`, use any version
	atIndex := strings.Index(arg, "@")
	if atIndex > 0 {
		parts := strings.SplitN(arg, "@", 2)
		path = parts[0]
		constraint = parts[1]
	}

	// split on colon if there is a network location
	colonIndex := strings.Index(arg, ":")
	if colonIndex > 0 {
		parts := strings.SplitN(arg, ":", 2)
		path = parts[0]
		source = parts[1]
	}

	return path, source, constraint
}

// deduceConstraint tries to puzzle out what kind of version is given in a string -
// semver, a revision, or as a fallback, a plain tag
func deduceConstraint(s string, pi gps.ProjectIdentifier, sm gps.SourceManager) (gps.Constraint, error) {
	if s == "" {
		// Find the default branch
		versions, err := sm.ListVersions(pi)
		if err != nil {
			return nil, errors.Wrapf(err, "list versions for %s(%s)", pi.ProjectRoot, pi.Source) // means repo does not exist
		}

		gps.SortPairedForUpgrade(versions)
		for _, v := range versions {
			if v.Type() == gps.IsBranch {
				return v.Unpair(), nil
			}
		}
	}

	// always semver if we can
	c, err := gps.NewSemverConstraintIC(s)
	if err == nil {
		return c, nil
	}

	slen := len(s)
	if slen == 40 {
		if _, err = hex.DecodeString(s); err == nil {
			// Whether or not it's intended to be a SHA1 digest, this is a
			// valid byte sequence for that, so go with Revision. This
			// covers git and hg
			return gps.Revision(s), nil
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
			return gps.NewVersion(s), nil
		}

		i2 := strings.LastIndex(s[:i3], "-")
		if _, err = strconv.ParseUint(s[i2+1:i3], 10, 64); err == nil {
			// Getting this far means it'd pretty much be nuts if it's not a
			// bzr rev, so don't bother parsing the email.
			return gps.Revision(s), nil
		}
	}

	// call out to network and get the package's versions
	versions, err := sm.ListVersions(pi)
	if err != nil {
		return nil, errors.Wrapf(err, "list versions for %s(%s)", pi.ProjectRoot, pi.Source) // means repo does not exist
	}

	for _, version := range versions {
		if s == version.String() {
			return version.Unpair(), nil
		}
	}
	return nil, errors.Errorf("%s is not a valid version for the package %s(%s)", s, pi.ProjectRoot, pi.Source)
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
