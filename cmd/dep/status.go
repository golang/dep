// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"sort"
	"strings"
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
	fs.BoolVar(&cmd.json, "json", false, "output in JSON format")
	fs.StringVar(&cmd.template, "f", "", "output in text/template format")
	fs.BoolVar(&cmd.dot, "dot", false, "output the dependency graph in GraphViz format")
	fs.BoolVar(&cmd.old, "old", false, "only show out-of-date dependencies")
	fs.BoolVar(&cmd.missing, "missing", false, "only show missing dependencies")
}

type statusCommand struct {
	json     bool
	template string
	output   string
	dot      bool
	old      bool
	missing  bool
}

type outputter interface {
	BasicHeader() error
	BasicLine(*BasicStatus) error
	BasicFooter() error
	MissingHeader() error
	MissingLine(*MissingStatus) error
	MissingFooter() error
}

type tableOutput struct{ w *tabwriter.Writer }

func (out *tableOutput) BasicHeader() error {
	_, err := fmt.Fprintf(out.w, "PROJECT\tCONSTRAINT\tVERSION\tREVISION\tLATEST\tPKGS USED\n")
	return err
}

func (out *tableOutput) BasicFooter() error {
	return out.w.Flush()
}

func (out *tableOutput) BasicLine(bs *BasicStatus) error {
	_, err := fmt.Fprintf(out.w,
		"%s\t%s\t%s\t%s\t%s\t%d\t\n",
		bs.ProjectRoot,
		bs.getConsolidatedConstraint(),
		formatVersion(bs.Version),
		formatVersion(bs.Revision),
		bs.getConsolidatedLatest(shortRev),
		bs.PackageCount,
	)
	return err
}

func (out *tableOutput) MissingHeader() error {
	_, err := fmt.Fprintln(out.w, "PROJECT\tMISSING PACKAGES")
	return err
}

func (out *tableOutput) MissingLine(ms *MissingStatus) error {
	_, err := fmt.Fprintf(out.w,
		"%s\t%s\t\n",
		ms.ProjectRoot,
		ms.MissingPackages,
	)
	return err
}

func (out *tableOutput) MissingFooter() error {
	return out.w.Flush()
}

type jsonOutput struct {
	w       io.Writer
	basic   []*rawStatus
	missing []*MissingStatus
}

func (out *jsonOutput) BasicHeader() error {
	out.basic = []*rawStatus{}
	return nil
}

func (out *jsonOutput) BasicFooter() error {
	return json.NewEncoder(out.w).Encode(out.basic)
}

func (out *jsonOutput) BasicLine(bs *BasicStatus) error {
	out.basic = append(out.basic, bs.marshalJSON())
	return nil
}

func (out *jsonOutput) MissingHeader() error {
	out.missing = []*MissingStatus{}
	return nil
}

func (out *jsonOutput) MissingLine(ms *MissingStatus) error {
	out.missing = append(out.missing, ms)
	return nil
}

func (out *jsonOutput) MissingFooter() error {
	return json.NewEncoder(out.w).Encode(out.missing)
}

type dotOutput struct {
	w io.Writer
	o string
	g *graphviz
	p *dep.Project
}

func (out *dotOutput) BasicHeader() error {
	out.g = new(graphviz).New()

	ptree, err := out.p.ParseRootPackageTree()
	// TODO(sdboyer) should be true, true, false, out.p.Manifest.IgnoredPackages()
	prm, _ := ptree.ToReachMap(true, false, false, nil)

	out.g.createNode(string(out.p.ImportRoot), "", prm.FlattenFn(paths.IsStandardImportPath))

	return err
}

func (out *dotOutput) BasicFooter() error {
	gvo := out.g.output()
	_, err := fmt.Fprintf(out.w, gvo.String())
	return err
}

func (out *dotOutput) BasicLine(bs *BasicStatus) error {
	out.g.createNode(bs.ProjectRoot, bs.getConsolidatedVersion(), bs.Children)
	return nil
}

func (out *dotOutput) MissingHeader() error                { return nil }
func (out *dotOutput) MissingLine(ms *MissingStatus) error { return nil }
func (out *dotOutput) MissingFooter() error                { return nil }

type templateOutput struct {
	w    io.Writer
	tmpl *template.Template
}

func (out *templateOutput) BasicHeader() error { return nil }
func (out *templateOutput) BasicFooter() error { return nil }

func (out *templateOutput) BasicLine(bs *BasicStatus) error {
	return out.tmpl.Execute(out.w, bs)
}

func (out *templateOutput) MissingHeader() error { return nil }
func (out *templateOutput) MissingFooter() error { return nil }

func (out *templateOutput) MissingLine(ms *MissingStatus) error {
	return out.tmpl.Execute(out.w, ms)
}

func (cmd *statusCommand) Run(ctx *dep.Ctx, args []string) error {
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

	if err := dep.ValidateProjectRoots(ctx, p.Manifest, sm); err != nil {
		return err
	}

	if len(args) > 0 {
		return runProjectStatus(ctx, args, p, sm)
	}

	var buf bytes.Buffer
	var out outputter
	switch {
	case cmd.missing:
		return errors.Errorf("not implemented")
	case cmd.old:
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
	case cmd.template != "":
		tmpl, err := template.New("status").Parse(cmd.template)
		if err != nil {
			return err
		}
		out = &templateOutput{
			w:    &buf,
			tmpl: tmpl,
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

func (cmd *statusCommand) validateFlags() error {
	// Operating mode flags.
	opModes := []string{}

	if cmd.old {
		opModes = append(opModes, "-old")
	}

	if cmd.missing {
		opModes = append(opModes, "-missing")
	}

	// Check if any other flags are passed with -dot.
	if cmd.dot {
		if cmd.template != "" {
			return errors.New("cannot pass template string with -dot")
		}

		if cmd.json {
			return errors.New("cannot pass multiple output format flags")
		}

		if len(opModes) > 0 {
			return errors.New("-dot generates dependency graph; cannot pass other flags")
		}
	}

	if len(opModes) > 1 {
		// List the flags because which flags are for operation mode might not
		// be apparent to the users.
		return errors.Wrapf(errors.New("cannot pass multiple operating mode flags"), "%v", opModes)
	}

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

	// Errors while collecting constraints should not fail the whole status run.
	// It should count the error and tell the user about incomplete results.
	cm, ccerrs := collectConstraints(ctx, p, sm)
	if len(ccerrs) > 0 {
		errCount += len(ccerrs)
	}

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

		if err := out.BasicHeader(); err != nil {
			return false, 0, err
		}

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
				} else if pp, has := p.Manifest.Constraints[proj.Ident().ProjectRoot]; has && pp.Constraint != nil {
					// If the manifest has a constraint then set that as the constraint.
					bs.Constraint = pp.Constraint
				} else {
					bs.Constraint = gps.Any()
					for _, c := range cm[bs.ProjectRoot] {
						bs.Constraint = c.Constraint.Intersect(bs.Constraint)
					}
				}

				// Only if we have a non-rev and non-plain version do/can we display
				// anything wrt the version's updateability.
				if bs.Version != nil && bs.Version.Type() != gps.IsVersion {
					c, has := p.Manifest.Constraints[proj.Ident().ProjectRoot]
					if !has {
						// Get constraint for locked project
						for _, lockedP := range p.Lock.P {
							if lockedP.Ident().ProjectRoot == proj.Ident().ProjectRoot {
								// Use the unpaired version as the constraint for checking updates.
								c.Constraint = bs.Version
							}
						}
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
			errCount += len(errListVerCh)
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
			if err := out.BasicLine(bsMap[string(proj.Ident().ProjectRoot)]); err != nil {
				return false, 0, err
			}
		}

		if footerErr := out.BasicFooter(); footerErr != nil {
			return false, 0, footerErr
		}

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

	if err = out.MissingHeader(); err != nil {
		return false, 0, err
	}

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
		err := out.MissingLine(&MissingStatus{ProjectRoot: string(root), MissingPackages: pkgs})
		if err != nil {
			return false, 0, err
		}
	}
	if err = out.MissingFooter(); err != nil {
		return false, 0, err
	}

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

// projectConstraint stores ProjectRoot and Constraint for that project.
type projectConstraint struct {
	Project    gps.ProjectRoot
	Constraint gps.Constraint
}

func (pc projectConstraint) String() string {
	return fmt.Sprintf("%s(%s)", pc.Constraint.String(), string(pc.Project))
}

// constraintsCollection is a map of ProjectRoot(dependency) and a collection of
// projectConstraint for the dependencies. This can be used to find constraints
// on a dependency and the projects that apply those constraints.
type constraintsCollection map[string][]projectConstraint

// collectConstraints collects constraints declared by all the dependencies.
// It returns constraintsCollection and a slice of errors encountered while
// collecting the constraints, if any.
func collectConstraints(ctx *dep.Ctx, p *dep.Project, sm gps.SourceManager) (constraintsCollection, []error) {
	logger := ctx.Err
	if !ctx.Verbose {
		logger = log.New(ioutil.Discard, "", 0)
	}

	logger.Println("Collecting project constraints:")

	var mutex sync.Mutex
	constraintCollection := make(constraintsCollection)

	// Get direct deps of the root project.
	_, directDeps, err := getDirectDependencies(sm, p)
	if err != nil {
		// Return empty collection, not nil, if we fail here.
		return constraintCollection, []error{errors.Wrap(err, "failed to get direct dependencies")}
	}

	// Create a root analyzer.
	rootAnalyzer := newRootAnalyzer(true, ctx, directDeps, sm)

	lp := p.Lock.Projects()

	// Channel for receiving all the errors.
	errCh := make(chan error, len(lp))

	var wg sync.WaitGroup

	// Iterate through the locked projects and collect constraints of all the projects.
	for i, proj := range lp {
		wg.Add(1)
		logger.Printf("(%d/%d) %s\n", i+1, len(lp), proj.Ident().ProjectRoot)

		go func(proj gps.LockedProject) {
			defer wg.Done()

			manifest, _, err := sm.GetManifestAndLock(proj.Ident(), proj.Version(), rootAnalyzer)
			if err != nil {
				errCh <- errors.Wrap(err, "error getting manifest and lock")
				return
			}

			// Get project constraints.
			pc := manifest.DependencyConstraints()

			// Obtain a lock for constraintCollection.
			mutex.Lock()
			defer mutex.Unlock()
			// Iterate through the project constraints to get individual dependency
			// project and constraint values.
			for pr, pp := range pc {
				tempCC := append(
					constraintCollection[string(pr)],
					projectConstraint{proj.Ident().ProjectRoot, pp.Constraint},
				)

				// Sort the inner projectConstraint slice by Project string.
				// Required for consistent returned value.
				sort.Sort(byProject(tempCC))
				constraintCollection[string(pr)] = tempCC
			}
		}(proj)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	if len(errCh) > 0 {
		for e := range errCh {
			errs = append(errs, e)
			logger.Println(e.Error())
		}
	}

	return constraintCollection, errs
}

type byProject []projectConstraint

func (p byProject) Len() int           { return len(p) }
func (p byProject) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p byProject) Less(i, j int) bool { return p[i].Project < p[j].Project }

// pubVersion type to store Public Version data of a project.
type pubVersions map[string][]string

// TabString returns a tabwriter compatible string of pubVersion.
func (pv pubVersions) TabString() string {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	// Create a list of version categories and sort it for consistent results.
	var catgs []string

	for catg := range pv {
		catgs = append(catgs, catg)
	}

	// Sort the list of categories.
	sort.Strings(catgs)

	// Count the number of different version categories. Use this count to add
	// a newline("\n") and tab("\t") in all the version string list except the
	// first one. This is required to maintain the indentation of the strings
	// when used with tabwriter.
	// semver:...\n \tbranches:...\n \tnonsemvers:...
	count := 0
	for _, catg := range catgs {
		count++
		if count > 1 {
			fmt.Fprintf(w, "\n \t")
		}

		vers := pv[catg]

		// Sort the versions list for consistent result.
		sort.Strings(vers)

		fmt.Fprintf(w, "%s: %s", catg, strings.Join(vers, ", "))
	}
	w.Flush()

	return buf.String()
}

// projectImporters stores a map of project names that import a specific project.
type projectImporters map[string]bool

func (pi projectImporters) String() string {
	var projects []string

	for p := range pi {
		projects = append(projects, p)
	}

	// Sort the projects for a consistent result.
	sort.Strings(projects)

	return strings.Join(projects, ", ")
}

// packageImporters stores a map of package and projects that import them.
type packageImporters map[string][]string

func (pi packageImporters) TabString() string {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	// Create a list of packages in the map and sort it for consistent results.
	var pkgs []string

	for pkg := range pi {
		pkgs = append(pkgs, pkg)
	}

	// Sort the list of packages.
	sort.Strings(pkgs)

	// Count the number of different packages. Use this count to add
	// a newline("\n") and tab("\t") in all the package string header except the
	// first one. This is required to maintain the indentation of the strings
	// when used with tabwriter.
	// github.com/x/y\n \t  github.com/a/b/foo\n \t  github.com/a/b/bar
	count := 0
	for _, pkg := range pkgs {
		count++
		if count > 1 {
			fmt.Fprintf(w, "\n \t")
		}

		fmt.Fprintf(w, "%s", pkg)

		importers := pi[pkg]

		// Sort the importers list for consistent result.
		sort.Strings(importers)

		for _, p := range importers {
			fmt.Fprintf(w, "\n \t  %s", p)
		}
	}
	w.Flush()

	return buf.String()
}

// projectConstraints is a slice of projectConstraint
type projectConstraints []projectConstraint

func (pcs projectConstraints) TabString() string {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	// Sort for consistent result.
	sort.Sort(byProject(pcs))

	// Count lines and add newlines("\n") and tabs("\t"), compatible with
	// tabwriter.
	// ^0.5.0(btb.com/x/y)\n \t^1.0.0(gh.com/f/b)\t \t^1.5.0(gh.com/a/c)
	count := 0
	for _, c := range pcs {
		count++
		if count > 1 {
			fmt.Fprintf(w, "\n \t")
		}

		fmt.Fprintf(w, "%s", c)
	}
	w.Flush()

	return buf.String()
}

type projectStatus struct {
	Project               string
	Version               string
	Constraints           projectConstraints
	Source                string
	AltSource             string
	PubVersions           pubVersions
	Revision              string
	LatestAllowed         string
	SourceType            string
	Packages              []string
	ProjectImporters      projectImporters
	PackageImporters      packageImporters
	UpstreamExists        bool
	UpstreamVersionExists bool
}

func (ps projectStatus) String() string {
	var buf bytes.Buffer

	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)

	upstreamExists := "no"
	if ps.UpstreamExists {
		upstreamExists = "yes"
	}

	upstreamVersionExists := "no"
	if ps.UpstreamVersionExists {
		upstreamVersionExists = "yes"
	}

	fmt.Fprintf(w, "\n"+
		"PROJECT:\t%s\n"+
		"VERSION:\t%s\n"+
		"CONSTRAINTS:\t%s\n"+
		"SOURCE:\t%s\n"+
		"ALT SOURCE:\t%s\n"+
		"PUB VERSION:\t%s\n"+
		"REVISION:\t%s\n"+
		"LATEST ALLOWED:\t%s\n"+
		"SOURCE TYPE:\t%s\n"+
		"PACKAGES:\t%s\n"+
		"PROJECT IMPORTERS:\t%s\n"+
		"PACKAGE IMPORTERS:\t%s\n"+
		"UPSTREAM EXISTS:\t%s\n"+
		"UPSTREAM VERSION EXISTS:\t%s",
		ps.Project, ps.Version, ps.Constraints.TabString(), ps.Source, ps.AltSource,
		ps.PubVersions.TabString(), ps.Revision, ps.LatestAllowed, ps.SourceType,
		strings.Join(ps.Packages, ", "), ps.ProjectImporters,
		ps.PackageImporters.TabString(), upstreamExists, upstreamVersionExists,
	)
	w.Flush()

	return buf.String()
}

func runProjectStatus(ctx *dep.Ctx, args []string, p *dep.Project, sm gps.SourceManager) error {
	// Collect all the project roots from the arguments.
	var prs []gps.ProjectRoot

	// Collect pointers to resultant projectStatus.
	var resultStatus []*projectStatus

	// Get the proper project root of the projects.
	for _, arg := range args {
		pr, err := sm.DeduceProjectRoot(arg)
		if err != nil {
			return err
		}

		// Check if the target project is in the lock.
		if !p.Lock.HasProjectWithRoot(pr) {
			return errors.Errorf("%s is not in the lock file. Ensure that the project is being used and run `dep ensure` to generate a new lock file.", pr)
		}

		prs = append(prs, pr)
	}

	// Collect all the constraints.
	cc, ccerrs := collectConstraints(ctx, p, sm)
	// If found any errors, print to stderr.
	if len(ccerrs) > 0 {
		if ctx.Verbose {
			for _, e := range ccerrs {
				ctx.Err.Println(e)
			}
		} else {
			ctx.Out.Println("Got errors while collecting constraints. Rerun with `-v` flag to see details.")
		}
		return errors.New("errors while collecting constraints")
	}

	// Collect list of packages in target projects.
	pkgs := make(map[gps.ProjectRoot][]string)

	// Collect reachmap of all the locked projects.
	reachmaps := make(map[string]pkgtree.ReachMap)

	// Create a list of depgraph packages to be used to exclude packages that
	// the root project does not use.
	rootPkgTree, err := p.ParseRootPackageTree()
	if err != nil {
		return err
	}
	rootrm, _ := rootPkgTree.ToReachMap(true, true, false, p.Manifest.IgnoredPackages())
	depgraphPkgs := rootrm.FlattenFn(paths.IsStandardImportPath)

	// Collect and store all the necessary data from pkgtree(s).
	// TODO: Make this concurrent.
	for _, pl := range p.Lock.Projects() {
		pkgtree, err := sm.ListPackages(pl.Ident(), pl.Version())
		if err != nil {
			return err
		}

		// Collect reachmaps.
		prm, _ := pkgtree.ToReachMap(true, true, false, p.Manifest.IgnoredPackages())
		reachmaps[string(pl.Ident().ProjectRoot)] = prm

		// Collect list of packages if it's one of the target projects.
		for _, pr := range prs {
			if pr == pl.Ident().ProjectRoot {
				for pkg := range pkgtree.Packages {
					pkgs[pr] = append(pkgs[pr], pkg)
				}
			}
		}
	}

	for _, pr := range prs {
		// Create projectStatus and add project name and source.
		projStatus := projectStatus{
			Project: string(pr),
			Source:  string(pr),
		}
		resultStatus = append(resultStatus, &projStatus)

		// Gather PROJECT IMPORTERS & PACKAGE IMPORTERS data.
		for projectroot, rmap := range reachmaps {
			// If it's not the target project, check if it imports the target
			// project.
			if string(pr) == projectroot {
				continue
			}

			for pkg, ie := range rmap {
				// Exclude packages not used by root project.
				if !contains(depgraphPkgs, pkg) {
					continue
				}

				// Iterate through the external imports and check if they
				// import any package from the target project.
				for _, p := range ie.External {
					if strings.HasPrefix(p, string(pr)) {
						// Initialize ProjectImporters map if it's the first entry.
						if len(projStatus.ProjectImporters) == 0 {
							projStatus.ProjectImporters = make(map[string]bool)
						}
						// Add to ProjectImporters if it doesn't exists.
						if _, ok := projStatus.ProjectImporters[projectroot]; !ok {
							projStatus.ProjectImporters[projectroot] = true
						}
						// Initialize PackageImporters map if it's the first entry.
						if len(projStatus.PackageImporters[p]) == 0 {
							projStatus.PackageImporters = make(map[string][]string)
						}
						// List Packages that import packages from target project.
						projStatus.PackageImporters[p] = append(projStatus.PackageImporters[p], pkg)
					}
				}
			}
		}

		// Gather data from the locked project.
		for _, pl := range p.Lock.Projects() {
			if pr != pl.Ident().ProjectRoot {
				continue
			}

			// VERSION
			projStatus.Version = pl.Version().String()
			// ALT SOURCE
			projStatus.AltSource = pl.Ident().Source
			// REVISION
			projStatus.Revision, _, _ = gps.VersionComponentStrings(pl.Version())

			srcType, err := sm.GetSourceType(pl.Ident())
			if err != nil {
				return err
			}
			// SOURCE TYPE
			projStatus.SourceType = srcType.String()

			existsUpstream, err := sm.ExistsUpstream(pl.Ident())
			if err != nil {
				return err
			}
			// UPSTREAM EXISTS
			projStatus.UpstreamExists = existsUpstream

			// Fetch all the versions.
			pvs, err := sm.ListVersions(pl.Ident())
			if err != nil {
				return err
			}

			// UPSTREAM VERSION EXISTS
			for _, pv := range pvs {
				if pv.Unpair().String() == pl.Version().String() {
					projStatus.UpstreamVersionExists = true
					break
				}
			}

			// PACKAGES
			projStatus.Packages = pkgs[pr]

			// PUB VERSION
			var semvers, branches, nonsemvers []string
			for _, pv := range pvs {
				switch pv.Type() {
				case gps.IsSemver:
					semvers = append(semvers, pv.Unpair().String())
				case gps.IsBranch:
					branches = append(branches, pv.Unpair().String())
				default:
					nonsemvers = append(nonsemvers, pv.Unpair().String())
				}
			}
			projStatus.PubVersions = make(map[string][]string)
			projStatus.PubVersions["semvers"] = semvers
			projStatus.PubVersions["branches"] = branches
			projStatus.PubVersions["nonsemvers"] = nonsemvers

			// CONSTRAINTS
			constraints := cc[string(pr)]
			for _, c := range constraints {
				projStatus.Constraints = append(projStatus.Constraints, c)
			}
		}
	}

	// LATEST ALLOWED
	// Perform a solve to get the latest allowed revisions.
	params := p.MakeParams()
	if ctx.Verbose {
		params.TraceLogger = ctx.Err
	}

	params.RootPackageTree, err = p.ParseRootPackageTree()
	if err != nil {
		return err
	}

	// Add all the target projects in params.ToChange.
	params.ToChange = append(params.ToChange, prs...)

	solver, err := gps.Prepare(params, sm)
	if err != nil {
		return err
	}

	solution, err := solver.Solve(context.TODO())
	if err != nil {
		return err
	}

	// Iterate through the solution and get the revisions for the target projects.
	projects := solution.Projects()
	for _, rs := range resultStatus {
		for _, project := range projects {
			if gps.ProjectRoot(rs.Project) == project.Ident().ProjectRoot {
				r, _, _ := gps.VersionComponentStrings(project.Version())
				rs.LatestAllowed = r
			}
		}
		ctx.Out.Println(rs)
	}

	return nil
}
