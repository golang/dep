// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sort"
	"sync"
	"text/tabwriter"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/paths"
	"github.com/golang/dep/gps/pkgtree"
	"github.com/pkg/errors"
)

const statusShortHelp = `Report the status of the project's dependencies`
const statusLongHelp = `
With no arguments, print the status of each dependency of the project.

  PROJECT     Import path
  CONSTRAINT  Version constraint, from the manifest
  VERSION     Version chosen, from the lock
  REVISION    VCS revision of the chosen version
  LATEST      Latest VCS revision available
  PKGS USED   Number of packages from this project that are actually used

With one or more explicitly specified packages, or with the -detailed flag,
print an extended status output for each dependency of the project.

  TODO    Another column description
  FOOBAR  Another column description

Status returns exit code zero if all dependencies are in a "good state".
`

const (
	shortRev uint8 = iota
	longRev
)

var (
	errFailedUpdate        = errors.New("failed to fetch updates")
	errFailedListPkg       = errors.New("failed to list packages")
	errMultipleFailures    = errors.New("multiple sources of failure")
	errInputDigestMismatch = errors.New("input-digest mismatch")
)

func (cmd *statusCommand) Name() string      { return "status" }
func (cmd *statusCommand) Args() string      { return "[package...]" }
func (cmd *statusCommand) ShortHelp() string { return statusShortHelp }
func (cmd *statusCommand) LongHelp() string  { return statusLongHelp }
func (cmd *statusCommand) Hidden() bool      { return false }

func (cmd *statusCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.detailed, "detailed", false, "report more detailed status")
	fs.BoolVar(&cmd.json, "json", false, "output in JSON format")
	fs.StringVar(&cmd.template, "f", "", "output in text/template format")
	fs.BoolVar(&cmd.dot, "dot", false, "output the dependency graph in GraphViz format")
	fs.BoolVar(&cmd.old, "old", false, "only show out-of-date dependencies")
	fs.BoolVar(&cmd.missing, "missing", false, "only show missing dependencies")
	fs.BoolVar(&cmd.unused, "unused", false, "only show unused dependencies")
	fs.BoolVar(&cmd.modified, "modified", false, "only show modified dependencies")
}

type statusCommand struct {
	detailed bool
	json     bool
	template string
	output   string
	dot      bool
	old      bool
	missing  bool
	unused   bool
	modified bool
}

type outputter interface {
	BasicHeader()
	BasicLine(*BasicStatus)
	BasicFooter()
	MissingHeader()
	MissingLine(*MissingStatus)
	MissingFooter()
}

type tableOutput struct{ w *tabwriter.Writer }

func (out *tableOutput) BasicHeader() {
	fmt.Fprintf(out.w, "PROJECT\tCONSTRAINT\tVERSION\tREVISION\tLATEST\tPKGS USED\n")
}

func (out *tableOutput) BasicFooter() {
	out.w.Flush()
}

func (out *tableOutput) BasicLine(bs *BasicStatus) {
	fmt.Fprintf(out.w,
		"%s\t%s\t%s\t%s\t%s\t%d\t\n",
		bs.ProjectRoot,
		bs.getConsolidatedConstraint(),
		formatVersion(bs.Version),
		formatVersion(bs.Revision),
		bs.getConsolidatedLatest(shortRev),
		bs.PackageCount,
	)
}

func (out *tableOutput) MissingHeader() {
	fmt.Fprintln(out.w, "PROJECT\tMISSING PACKAGES")
}

func (out *tableOutput) MissingLine(ms *MissingStatus) {
	fmt.Fprintf(out.w,
		"%s\t%s\t\n",
		ms.ProjectRoot,
		ms.MissingPackages,
	)
}

func (out *tableOutput) MissingFooter() {
	out.w.Flush()
}

type jsonOutput struct {
	w       io.Writer
	basic   []*rawStatus
	missing []*MissingStatus
}

func (out *jsonOutput) BasicHeader() {
	out.basic = []*rawStatus{}
}

func (out *jsonOutput) BasicFooter() {
	json.NewEncoder(out.w).Encode(out.basic)
}

func (out *jsonOutput) BasicLine(bs *BasicStatus) {
	out.basic = append(out.basic, bs.marshalJSON())
}

func (out *jsonOutput) MissingHeader() {
	out.missing = []*MissingStatus{}
}

func (out *jsonOutput) MissingLine(ms *MissingStatus) {
	out.missing = append(out.missing, ms)
}

func (out *jsonOutput) MissingFooter() {
	json.NewEncoder(out.w).Encode(out.missing)
}

type dotOutput struct {
	w io.Writer
	o string
	g *graphviz
	p *dep.Project
}

func (out *dotOutput) BasicHeader() {
	out.g = new(graphviz).New()

	ptree, _ := out.p.ParseRootPackageTree()
	// TODO(sdboyer) should be true, true, false, out.p.Manifest.IgnoredPackages()
	prm, _ := ptree.ToReachMap(true, false, false, nil)

	out.g.createNode(string(out.p.ImportRoot), "", prm.FlattenFn(paths.IsStandardImportPath))
}

func (out *dotOutput) BasicFooter() {
	gvo := out.g.output()
	fmt.Fprintf(out.w, gvo.String())
}

func (out *dotOutput) BasicLine(bs *BasicStatus) {
	out.g.createNode(bs.ProjectRoot, bs.getConsolidatedVersion(), bs.Children)
}

func (out *dotOutput) MissingHeader()                {}
func (out *dotOutput) MissingLine(ms *MissingStatus) {}
func (out *dotOutput) MissingFooter()                {}

func (cmd *statusCommand) Run(ctx *dep.Ctx, args []string) error {
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

	if err := dep.ValidateProjectRoots(ctx, p.Manifest, sm); err != nil {
		return err
	}

	var buf bytes.Buffer
	var out outputter
	switch {
	case cmd.modified:
		return errors.Errorf("not implemented")
	case cmd.unused:
		return errors.Errorf("not implemented")
	case cmd.missing:
		return errors.Errorf("not implemented")
	case cmd.old:
		return errors.Errorf("not implemented")
	case cmd.detailed:
		return errors.Errorf("not implemented")
	case cmd.json:
		out = &jsonOutput{
			w: &buf,
		}
	case cmd.dot:
		out = &dotOutput{
			p: p,
			o: cmd.output,
			w: &buf,
		}
	default:
		out = &tableOutput{
			w: tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0),
		}
	}

	// Check if the lock file exists.
	if p.Lock == nil {
		return errors.Errorf("no Gopkg.lock found. Run `dep ensure` to generate lock file")
	}

	hasMissingPkgs, errCount, err := runStatusAll(ctx, out, p, sm)
	if err != nil {
		switch err {
		case errFailedUpdate:
			// Print the results with unknown data
			ctx.Out.Println(buf.String())
			// Print the help when in non-verbose mode
			if !ctx.Verbose {
				ctx.Out.Printf("The status of %d projects are unknown due to errors. Rerun with `-v` flag to see details.\n", errCount)
			}
		case errInputDigestMismatch:
			// Tell the user why mismatch happened and how to resolve it.
			if hasMissingPkgs {
				ctx.Err.Printf("Lock inputs-digest mismatch due to the following packages missing from the lock:\n\n")
				ctx.Out.Print(buf.String())
				ctx.Err.Printf("\nThis happens when a new import is added. Run `dep ensure` to install the missing packages.\n")
			} else {
				ctx.Err.Printf("Lock inputs-digest mismatch. This happens when Gopkg.toml is modified.\n" +
					"Run `dep ensure` to regenerate the inputs-digest.")
			}
		}

		return err
	}

	// Print the status output
	ctx.Out.Print(buf.String())

	return nil
}

type rawStatus struct {
	ProjectRoot  string
	Constraint   string
	Version      string
	Revision     string
	Latest       string
	PackageCount int
}

// BasicStatus contains all the information reported about a single dependency
// in the summary/list status output mode.
type BasicStatus struct {
	ProjectRoot  string
	Children     []string
	Constraint   gps.Constraint
	Version      gps.UnpairedVersion
	Revision     gps.Revision
	Latest       gps.Version
	PackageCount int
	hasOverride  bool
	hasError     bool
}

func (bs *BasicStatus) getConsolidatedConstraint() string {
	var constraint string
	if bs.Constraint != nil {
		if v, ok := bs.Constraint.(gps.Version); ok {
			constraint = formatVersion(v)
		} else {
			constraint = bs.Constraint.String()
		}
	}

	if bs.hasOverride {
		constraint += " (override)"
	}

	return constraint
}

func (bs *BasicStatus) getConsolidatedVersion() string {
	version := formatVersion(bs.Revision)
	if bs.Version != nil {
		version = formatVersion(bs.Version)
	}
	return version
}

func (bs *BasicStatus) getConsolidatedLatest(revSize uint8) string {
	latest := ""
	if bs.Latest != nil {
		switch revSize {
		case shortRev:
			latest = formatVersion(bs.Latest)
		case longRev:
			latest = bs.Latest.String()
		}
	}

	if bs.hasError {
		latest += "unknown"
	}

	return latest
}

func (bs *BasicStatus) marshalJSON() *rawStatus {
	return &rawStatus{
		ProjectRoot:  bs.ProjectRoot,
		Constraint:   bs.getConsolidatedConstraint(),
		Version:      formatVersion(bs.Version),
		Revision:     string(bs.Revision),
		Latest:       bs.getConsolidatedLatest(longRev),
		PackageCount: bs.PackageCount,
	}
}

// MissingStatus contains information about all the missing packages in a project.
type MissingStatus struct {
	ProjectRoot     string
	MissingPackages []string
}

func runStatusAll(ctx *dep.Ctx, out outputter, p *dep.Project, sm gps.SourceManager) (hasMissingPkgs bool, errCount int, err error) {
	// While the network churns on ListVersions() requests, statically analyze
	// code from the current project.
	ptree, err := p.ParseRootPackageTree()
	if err != nil {
		return false, 0, err
	}

	// Set up a solver in order to check the InputHash.
	params := gps.SolveParameters{
		ProjectAnalyzer: dep.Analyzer{},
		RootDir:         p.AbsRoot,
		RootPackageTree: ptree,
		Manifest:        p.Manifest,
		// Locks aren't a part of the input hash check, so we can omit it.
	}

	logger := ctx.Err
	if ctx.Verbose {
		params.TraceLogger = ctx.Err
	} else {
		logger = log.New(ioutil.Discard, "", 0)
	}

	if err := ctx.ValidateParams(sm, params); err != nil {
		return false, 0, err
	}

	s, err := gps.Prepare(params, sm)
	if err != nil {
		return false, 0, errors.Wrapf(err, "could not set up solver for input hashing")
	}

	cm := collectConstraints(ptree, p, sm)

	// Get the project list and sort it so that the printed output users see is
	// deterministically ordered. (This may be superfluous if the lock is always
	// written in alpha order, but it doesn't hurt to double down.)
	slp := p.Lock.Projects()
	sort.Slice(slp, func(i, j int) bool {
		return slp[i].Ident().Less(slp[j].Ident())
	})

	if bytes.Equal(s.HashInputs(), p.Lock.SolveMeta.InputsDigest) {
		// If these are equal, we're guaranteed that the lock is a transitively
		// complete picture of all deps. That eliminates the need for at least
		// some checks.

		out.BasicHeader()

		logger.Println("Checking upstream projects:")

		// BasicStatus channel to collect all the BasicStatus.
		bsCh := make(chan *BasicStatus, len(slp))

		// Error channels to collect different errors.
		errListPkgCh := make(chan error, len(slp))
		errListVerCh := make(chan error, len(slp))

		var wg sync.WaitGroup

		for i, proj := range slp {
			wg.Add(1)
			logger.Printf("(%d/%d) %s\n", i+1, len(slp), proj.Ident().ProjectRoot)

			go func(proj gps.LockedProject) {
				bs := BasicStatus{
					ProjectRoot:  string(proj.Ident().ProjectRoot),
					PackageCount: len(proj.Packages()),
				}

				// Get children only for specific outputers
				// in order to avoid slower status process.
				switch out.(type) {
				case *dotOutput:
					ptr, err := sm.ListPackages(proj.Ident(), proj.Version())

					if err != nil {
						bs.hasError = true
						errListPkgCh <- err
					}

					prm, _ := ptr.ToReachMap(true, true, false, p.Manifest.IgnoredPackages())
					bs.Children = prm.FlattenFn(paths.IsStandardImportPath)
				}

				// Split apart the version from the lock into its constituent parts.
				switch tv := proj.Version().(type) {
				case gps.UnpairedVersion:
					bs.Version = tv
				case gps.Revision:
					bs.Revision = tv
				case gps.PairedVersion:
					bs.Version = tv.Unpair()
					bs.Revision = tv.Revision()
				}

				// Check if the manifest has an override for this project. If so,
				// set that as the constraint.
				if pp, has := p.Manifest.Ovr[proj.Ident().ProjectRoot]; has && pp.Constraint != nil {
					bs.hasOverride = true
					bs.Constraint = pp.Constraint
				} else {
					bs.Constraint = gps.Any()
					for _, c := range cm[bs.ProjectRoot] {
						bs.Constraint = c.Intersect(bs.Constraint)
					}
				}

				// Only if we have a non-rev and non-plain version do/can we display
				// anything wrt the version's updateability.
				if bs.Version != nil && bs.Version.Type() != gps.IsVersion {
					c, has := p.Manifest.Constraints[proj.Ident().ProjectRoot]
					if !has {
						c.Constraint = gps.Any()
					}
					// TODO: This constraint is only the constraint imposed by the
					// current project, not by any transitive deps. As a result,
					// transitive project deps will always show "any" here.
					bs.Constraint = c.Constraint

					vl, err := sm.ListVersions(proj.Ident())
					if err == nil {
						gps.SortPairedForUpgrade(vl)

						for _, v := range vl {
							// Because we've sorted the version list for
							// upgrade, the first version we encounter that
							// matches our constraint will be what we want.
							if c.Constraint.Matches(v) {
								bs.Latest = v.Revision()
								break
							}
						}
					} else {
						// Failed to fetch version list (could happen due to
						// network issue).
						bs.hasError = true
						errListVerCh <- err
					}
				}

				bsCh <- &bs

				wg.Done()
			}(proj)
		}

		wg.Wait()
		close(bsCh)
		close(errListPkgCh)
		close(errListVerCh)

		// Newline after printing the status progress output.
		logger.Println()

		// List Packages errors. This would happen only for dot output.
		if len(errListPkgCh) > 0 {
			err = errFailedListPkg
			if ctx.Verbose {
				for err := range errListPkgCh {
					ctx.Err.Println(err.Error())
				}
				ctx.Err.Println()
			}
		}

		// List Version errors.
		if len(errListVerCh) > 0 {
			if err == nil {
				err = errFailedUpdate
			} else {
				err = errMultipleFailures
			}

			// Count ListVersions error because we get partial results when
			// this happens.
			errCount = len(errListVerCh)
			if ctx.Verbose {
				for err := range errListVerCh {
					ctx.Err.Println(err.Error())
				}
				ctx.Err.Println()
			}
		}

		// A map of ProjectRoot and *BasicStatus. This is used in maintain the
		// order of BasicStatus in output by collecting all the BasicStatus and
		// then using them in order.
		bsMap := make(map[string]*BasicStatus)
		for bs := range bsCh {
			bsMap[bs.ProjectRoot] = bs
		}

		// Use the collected BasicStatus in outputter.
		for _, proj := range slp {
			out.BasicLine(bsMap[string(proj.Ident().ProjectRoot)])
		}

		out.BasicFooter()

		return false, errCount, err
	}

	// Hash digest mismatch may indicate that some deps are no longer
	// needed, some are missing, or that some constraints or source
	// locations have changed.
	//
	// It's possible for digests to not match, but still have a correct
	// lock.
	rm, _ := ptree.ToReachMap(true, true, false, p.Manifest.IgnoredPackages())

	external := rm.FlattenFn(paths.IsStandardImportPath)
	roots := make(map[gps.ProjectRoot][]string, len(external))

	type fail struct {
		ex  string
		err error
	}
	var errs []fail
	for _, e := range external {
		root, err := sm.DeduceProjectRoot(e)
		if err != nil {
			errs = append(errs, fail{
				ex:  e,
				err: err,
			})
			continue
		}

		roots[root] = append(roots[root], e)
	}

	if len(errs) != 0 {
		// TODO this is just a fix quick so staticcheck doesn't complain.
		// Visually reconciling failure to deduce project roots with the rest of
		// the mismatch output is a larger problem.
		ctx.Err.Printf("Failed to deduce project roots for import paths:\n")
		for _, fail := range errs {
			ctx.Err.Printf("\t%s: %s\n", fail.ex, fail.err.Error())
		}

		return false, 0, errors.New("address issues with undeducible import paths to get more status information")
	}

	out.MissingHeader()

outer:
	for root, pkgs := range roots {
		// TODO also handle the case where the project is present, but there
		// are items missing from just the package list
		for _, lp := range slp {
			if lp.Ident().ProjectRoot == root {
				continue outer
			}
		}

		hasMissingPkgs = true
		out.MissingLine(&MissingStatus{ProjectRoot: string(root), MissingPackages: pkgs})
	}
	out.MissingFooter()

	// We are here because of an input-digest mismatch. Return error.
	return hasMissingPkgs, 0, errInputDigestMismatch
}

func formatVersion(v gps.Version) string {
	if v == nil {
		return ""
	}
	switch v.Type() {
	case gps.IsBranch:
		return "branch " + v.String()
	case gps.IsRevision:
		r := v.String()
		if len(r) > 7 {
			r = r[:7]
		}
		return r
	}
	return v.String()
}

func collectConstraints(ptree pkgtree.PackageTree, p *dep.Project, sm gps.SourceManager) map[string][]gps.Constraint {
	// TODO
	return map[string][]gps.Constraint{}
}
