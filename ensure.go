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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

const ensureShortHelp = `Ensure a dependency is vendored in the project`
const ensureLongHelp = `
Ensure is used to fetch project dependencies into the vendor folder, as well as
to set version constraints for specific dependencies. It takes user input,
solves the updated dependency graph of the project, writes any changes to the
manifest and lock file, and downloads dependencies to the vendor folder.

Package spec:

  <path>[:alt location][@<version specifier>]

Examples:

  dep ensure                                   Populate vendor from existing manifest and lock
  dep ensure github.com/heroku/rollrus@^0.9.1  Update a specific dependency to a specific version
  dep ensure -update                           Update all dependencies to latest permitted versions

For more detailed usage examples, see dep ensure -examples.
`
const ensureExamples = `
dep ensure

    Solve the project's dependency graph, and download all dependencies to the
    vendor folder. If a dependency is in the lock file, use the version
    specified there. Otherwise, use the most recent version that can satisfy the
    constraints in the manifest file.

dep ensure -update

    Update all dependencies to the latest version allowed by the manifest, ignoring
    any versions specified in the lock file. Update the lock file with any
    changes.

dep ensure github.com/heroku/rollrus

    Update a specific dependency to the latest version allowed by the manifest,
    including all of its transitive dependencies.

dep ensure github.com/heroku/rollrus@~0.9.0

    Same as above, but choose any release matching 0.9.x, preferring latest. If
    a constraint was previously set in the manifest, this resets it.

dep ensure github.com/heroku/rollrus@^0.9.1

    Same as above, but choose any release >= 0.9.1, < 1.0.0. This form of
    constraint strikes a good balance of safety and flexibility, and should be
    preferred for libraries.

dep ensure github.com/heroku/rollrus:git.internal.com/foo/bar

    Fetch the dependency from a different location.

dep ensure github.com/heroku/rollrus==1.2.3  # 1.2.3 exactly
dep ensure github.com/heroku/rollrus=^1.2.0  # >= 1.2.0, < 2.0.0

    Fetch the dependency at a specific version or range, and update the lock
    file, but don't update the manifest file. Will fail if the specified version
    doesn't satisfy the constraint in the manifest file.

dep ensure -override github.com/heroku/rollrus@^0.9.1

    Forcefully and transitively override any constraint for this dependency.
    This can inadvertantly make your dependency graph unsolvable; use sparingly.
`

func (cmd *ensureCommand) Name() string      { return "ensure" }
func (cmd *ensureCommand) Args() string      { return "[spec...]" }
func (cmd *ensureCommand) ShortHelp() string { return ensureShortHelp }
func (cmd *ensureCommand) LongHelp() string  { return ensureLongHelp }

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

	p, err := depContext.loadProject("")
	if err != nil {
		return err
	}

	sm, err := depContext.sourceManager()
	if err != nil {
		return err
	}
	defer sm.Release()

	var errs []error
	for _, arg := range args {
		// default persist to manifest
		pc, err := getProjectConstraint(arg, sm)
		if err != nil {
			errs = append(errs, err)
		}

		if gps.IsAny(pc.Constraint) && pc.Ident.NetworkName == "" {
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
			NetworkName: pc.Ident.NetworkName,
			Constraint:  pc.Constraint,
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
		}

		// Empty overrides are fine (in contrast to deps), because they actually
		// carry meaning - they force the constraints entirely open for a given
		// project. Inadvisable, but meaningful.

		p.m.Ovr[pc.Ident.ProjectRoot] = gps.ProjectProperties{
			NetworkName: pc.Ident.NetworkName,
			Constraint:  pc.Constraint,
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
		for err := range errs {
			fmt.Fprintln(&buf, err)
		}

		return errors.New(buf.String())
	}

	params := gps.SolveParameters{
		RootDir:  p.absroot,
		Manifest: p.m,
		Lock:     p.l,
	}
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

	p.l.P = solution.Projects()
	p.l.Memo = solution.InputHash()

	tv, err := ioutil.TempDir("", "vendor")
	if err != nil {
		return errors.Wrap(err, "ensure making temporary vendor")
	}
	defer os.RemoveAll(tv)

	tm, err := ioutil.TempFile("", "manifest")
	if err != nil {
		return errors.Wrap(err, "ensure making temporary manifest")
	}
	tm.Close()
	defer os.Remove(tm.Name())

	tl, err := ioutil.TempFile("", "lock")
	if err != nil {
		return errors.Wrap(err, "ensure making temporary lock file")
	}
	tl.Close()
	defer os.Remove(tl.Name())

	if err := gps.WriteDepTree(tv, p.l, sm, true); err != nil {
		return errors.Wrap(err, "ensure gps.WriteDepTree")
	}

	if err := writeFile(tm.Name(), p.m); err != nil {
		return errors.Wrap(err, "ensure writeFile for manifest")
	}

	if err := writeFile(tl.Name(), p.l); err != nil {
		return errors.Wrap(err, "ensure writeFile for lock")
	}

	if err := copyFile(tm.Name(), filepath.Join(p.absroot, manifestName)); err != nil {
		return errors.Wrap(err, "ensure moving temp manifest into place!")
	}
	os.Remove(tm.Name())

	if err := copyFile(tl.Name(), filepath.Join(p.absroot, lockName)); err != nil {
		return errors.Wrap(err, "ensure moving temp manifest into place!")
	}
	os.Remove(tl.Name())

	os.RemoveAll(filepath.Join(p.absroot, "vendor"))
	if err := copyFolder(tv, filepath.Join(p.absroot, "vendor")); err != nil {
		return errors.Wrap(err, "ensure moving temp vendor")
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
	atIndex := strings.Index(arg, "@")
	if atIndex > 0 {
		parts := strings.SplitN(arg, "@", 2)
		constraint.Constraint = deduceConstraint(parts[1])
		arg = parts[0]
	}
	// TODO: What if there is no @, assume default branch (which may not be master) ?
	// TODO: if we decide to keep equals.....

	// split on colon if there is a network location
	colonIndex := strings.Index(arg, ":")
	if colonIndex > 0 {
		parts := strings.SplitN(arg, ":", 2)
		arg = parts[0]
		constraint.Ident.NetworkName = parts[1]
	}

	pr, err := sm.DeduceProjectRoot(arg)
	if err != nil {
		return constraint, errors.Wrapf(err, "could not infer project root from dependency path: %s", arg) // this should go through to the user
	}

	if string(pr) != arg {
		return constraint, errors.Wrapf(err, "dependency path %s is not a project root", arg)
	}
	constraint.Ident.ProjectRoot = gps.ProjectRoot(arg)

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
