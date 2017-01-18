// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
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
	fs.BoolVar(&cmd.update, "update", false, "ensure all dependencies are at the latest version allowed by the manifest")
	fs.BoolVar(&cmd.dryRun, "n", false, "dry run, don't actually ensure anything")
	fs.Var(&cmd.overrides, "override", "specify an override constraint spec (repeatable)")
}

type ensureCommand struct {
	examples  bool
	update    bool
	dryRun    bool
	overrides stringSlice
}

func (cmd *ensureCommand) Run(args []string) error {
	if cmd.examples {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(ensureExamples))
		return nil
	}

	if cmd.update && len(args) > 0 {
		return errors.New("Cannot pass -update and itemized project list (for now)")
	}

	p, err := depContext.loadProject("")
	if err != nil {
		return err
	}

	sm, err := depContext.sourceManager()
	if err != nil {
		return err
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	var errs []error
	var requestedPackages []string
	for _, arg := range args {
		// default persist to manifest
		pc, err := getProjectConstraint(arg, sm)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		requestedPackages = append(requestedPackages, string(pc.Ident.ProjectRoot))

		if gps.IsAny(pc.Constraint) && pc.Ident.Source == "" {
			// If the input specified neither a network name nor a constraint,
			// then the strict thing to do would be to remove the entry
			// entirely. But that would probably be quite surprising for users,
			// and it's what rm is for, so just ignore the input.
			//
			// TODO(sdboyer): for this case - or just in general - do we want to
			// add project args to the requires list temporarily for this run?
			if _, has := p.m.Dependencies[pc.Ident.ProjectRoot]; !has {
				logf("No constraint or alternate source specified for %q, omitting from manifest", pc.Ident.ProjectRoot)
			}
			// If it's already in the manifest, no need to log
			continue
		}

		p.m.Dependencies[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			Source:     pc.Ident.Source,
			Constraint: pc.Constraint,
		}

		for i, lp := range p.l.P {
			if lp.Ident() == pc.Ident {
				p.l.P = append(p.l.P[:i], p.l.P[i+1:]...)
				break
			}
		}
	}

	for _, ovr := range cmd.overrides {
		pc, err := getProjectConstraint(ovr, sm)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// Empty overrides are fine (in contrast to deps), because they actually
		// carry meaning - they force the constraints entirely open for a given
		// project. Inadvisable, but meaningful.

		p.m.Ovr[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			Source:     pc.Ident.Source,
			Constraint: pc.Constraint,
		}

		for i, lp := range p.l.P {
			if lp.Ident() == pc.Ident {
				p.l.P = append(p.l.P[:i], p.l.P[i+1:]...)
				break
			}
		}
	}

	if len(errs) > 0 {
		var buf bytes.Buffer
		for _, err := range errs {
			fmt.Fprintln(&buf, err)
		}

		return errors.New(buf.String())
	}

	params := p.makeParams()
	// If -update was passed, we want the solver to allow all versions to change
	params.ChangeAll = cmd.update

	if *verbose {
		params.Trace = true
		params.TraceLogger = log.New(os.Stderr, "", 0)
	}

	params.RootPackageTree, err = gps.ListPackages(p.absroot, string(p.importroot))
	if err != nil {
		return errors.Wrap(err, "ensure ListPackage for project")
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

	// generate warning if the package specifically requested was not added to
	// the generated lock, this would mean it is not imported in their code
	for _, pkg := range requestedPackages {
		var found bool
		for _, lp := range solution.Projects() {
			if string(lp.Ident().ProjectRoot) == pkg {
				found = true
				break
			}
		}

		if !found {
			fmt.Fprintf(os.Stdout, "WARNING: %s was requested but is not imported in your code.\n", pkg)
		}
	}

	sw := safeWriter{
		root: p.absroot,
		m:    p.m,
		l:    p.l,
		nl:   solution,
		sm:   sm,
	}

	// check if vendor exists, because if the locks are the same but
	// vendor does not exist we should write vendor
	var writeV bool
	vendorExists, _ := isDir(filepath.Join(sw.root, "vendor"))
	if !vendorExists && solution != nil {
		writeV = true
	}

	return errors.Wrap(sw.writeAllSafe(writeV), "grouped write of manifest, lock and vendor")
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
		return constraint, fmt.Errorf("dependency path %s is not a project root, try %s instead", arg, pr)
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
					if pv, ok := version.(gps.PairedVersion); ok {
						version = pv.Unpair()
					}
					found = true
					constraint.Constraint = version
					break
				}
			}

			if !found {
				return constraint, fmt.Errorf("%s is not a valid version for the package %s", versionStr, arg)
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

// renameWithFallback attempts to rename a file or directory, but falls back to
// copying in the event of a cross-link device error. If the fallback copy
// succeeds, src is still removed, emulating normal rename behavior.
func renameWithFallback(src, dest string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// Windows cannot use syscall.Rename to rename a directory
	if runtime.GOOS == "windows" && fi.IsDir() {
		if err := copyFolder(src, dest); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}

	err = os.Rename(src, dest)
	if err == nil {
		return nil
	}

	terr, ok := err.(*os.LinkError)
	if !ok {
		return err
	}

	// Rename may fail if src and dest are on different devices; fall back to
	// copy if we detect that case. syscall.EXDEV is the common name for the
	// cross device link error which has varying output text across different
	// operating systems.
	var cerr error
	if terr.Err == syscall.EXDEV {
		vlogf("Cross link err (is temp dir on same partition as project?); falling back to manual copy: %s", terr)
		if fi.IsDir() {
			cerr = copyFolder(src, dest)
		} else {
			cerr = copyFile(src, dest)
		}
	} else if runtime.GOOS == "windows" {
		// In windows it can drop down to an operating system call that
		// returns an operating system error with a different number and
		// message. Checking for that as a fall back.
		noerr, ok := terr.Err.(syscall.Errno)
		// 0x11 (ERROR_NOT_SAME_DEVICE) is the windows error.
		// See https://msdn.microsoft.com/en-us/library/cc231199.aspx
		if ok && noerr == 0x11 {
			vlogf("Cross link err (is temp dir on same partition as project?); falling back to manual copy: %s", terr)
			cerr = copyFile(src, dest)
		}
	} else {
		return terr
	}

	if cerr != nil {
		return cerr
	}

	return os.RemoveAll(src)
}

// copyFolder takes in a directory and copies its contents to the destination.
// It preserves the file mode on files as well.
func copyFolder(src string, dest string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dest, fi.Mode())
	if err != nil {
		return err
	}

	dir, err := os.Open(src)
	if err != nil {
		return err
	}
	defer dir.Close()

	objects, err := dir.Readdir(-1)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		if obj.Mode()&os.ModeSymlink != 0 {
			continue
		}

		srcfile := filepath.Join(src, obj.Name())
		destfile := filepath.Join(dest, obj.Name())

		if obj.IsDir() {
			err = copyFolder(srcfile, destfile)
			if err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcfile, destfile); err != nil {
			return err
		}
	}

	return nil
}

// copyFile copies a file from one place to another with the permission bits
// perserved as well.
func copyFile(src string, dest string) error {
	srcfile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcfile.Close()

	destfile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destfile.Close()

	if _, err := io.Copy(destfile, srcfile); err != nil {
		return err
	}

	srcinfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dest, srcinfo.Mode())
}
