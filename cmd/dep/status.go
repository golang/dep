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
	"log"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/golang/dep"
	"github.com/sdboyer/gps"
	"hash/fnv"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func (cmd *statusCommand) Name() string      { return "status" }
func (cmd *statusCommand) Args() string      { return "[package...]" }
func (cmd *statusCommand) ShortHelp() string { return statusShortHelp }
func (cmd *statusCommand) LongHelp() string  { return statusLongHelp }
func (cmd *statusCommand) Hidden() bool      { return false }

func (cmd *statusCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.detailed, "detailed", false, "report more detailed status")
	fs.BoolVar(&cmd.json, "json", false, "output in JSON format")
	fs.StringVar(&cmd.template, "f", "", "output in text/template format")
	fs.StringVar(&cmd.output, "o", "output.svg", "output file")
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
	var constraint string
	if v, ok := bs.Constraint.(gps.Version); ok {
		constraint = formatVersion(v)
	} else {
		constraint = bs.Constraint.String()
	}
	fmt.Fprintf(out.w,
		"%s\t%s\t%s\t%s\t%s\t%d\t\n",
		bs.ProjectRoot,
		constraint,
		formatVersion(bs.Version),
		formatVersion(bs.Revision),
		formatVersion(bs.Latest),
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
	basic   []*BasicStatus
	missing []*MissingStatus
}

func (out *jsonOutput) BasicHeader() {
	out.basic = []*BasicStatus{}
}

func (out *jsonOutput) BasicFooter() {
	json.NewEncoder(out.w).Encode(out.basic)
}

func (out *jsonOutput) BasicLine(bs *BasicStatus) {
	out.basic = append(out.basic, bs)
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

type dotProject struct {
	project  string
	version  string
	children []string
}

func (dp *dotProject) hash() uint32 {
	h := fnv.New32a()
	h.Write([]byte(dp.project))
	return h.Sum32()
}

func (dp *dotProject) label() string {
	label := []string{dp.project}

	if dp.version != "" {
		label = append(label, dp.version)
	}

	return strings.Join(label, "\n")
}

type dotOutput struct {
	w   io.Writer
	o   string
	p   *dep.Project
	dps []*dotProject
	b   bytes.Buffer
	bsh map[string]uint32
}

func (out *dotOutput) BasicHeader() {

	// TODO Check for dot package before doing something
	// cmd := exec.Command("dot", "-V")

	out.dps = []*dotProject{}
	out.bsh = make(map[string]uint32)
	out.b.WriteString("digraph { node [shape=box]; ")

	ptree, _ := gps.ListPackages(out.p.AbsRoot, string(out.p.ImportRoot))
	prm, _ := ptree.ToReachMap(true, false, false, nil)

	rdp := &dotProject{
		project:  string(out.p.ImportRoot),
		version:  "",
		children: prm.Flatten(false),
	}

	out.dps = append(out.dps, rdp)
}

func (out *dotOutput) BasicFooter() {

	for _, dp := range out.dps {
		out.bsh[dp.project] = dp.hash()

		// Create name boxes, and name them using hashes
		// to avoid encoding name conflicts
		out.b.WriteString(fmt.Sprintf("%d [label=\"%s\"];", dp.hash(), dp.label()))
	}

	// Store relations to avoid duplication
	rels := make(map[string]bool)

	// Create relations
	for _, dp := range out.dps {
		for _, bsc := range dp.children {
			for pr, hsh := range out.bsh {
				if strings.HasPrefix(bsc, pr) {
					r := fmt.Sprintf("%d -> %d", out.bsh[dp.project], hsh)

					if _, ex := rels[r]; !ex {
						out.b.WriteString(r + "; ")
						rels[r] = true
					}

				}
			}
		}
	}

	out.b.WriteString("}")

	// TODO: Pipe created string to dot
	// Storing Graphviz output inside temp file to generate dot output
	tf, err := ioutil.TempFile(os.TempDir(), "")

	if err != nil {
		fmt.Printf("%v", err)
	}

	defer syscall.Unlink(tf.Name())
	ioutil.WriteFile(tf.Name(), out.b.Bytes(), 0644)

	if err := exec.Command("dot", tf.Name(), "-Tsvg", "-o", out.o).Run(); err != nil {
		fmt.Errorf("Something went wrong generating dot output: %s", err)
	}

	fmt.Fprintf(out.w, "Output generated and stored %s", out.o)
}

func (out *dotOutput) BasicLine(bs *BasicStatus) {

	dp := &dotProject{
		project:  bs.ProjectRoot,
		version:  bs.Version.String(),
		children: bs.Children,
	}

	out.dps = append(out.dps, dp)
}

func (out *dotOutput) MissingHeader()                {}
func (out *dotOutput) MissingLine(ms *MissingStatus) {}
func (out *dotOutput) MissingFooter()                {}

func (cmd *statusCommand) Run(ctx *dep.Ctx, args []string) error {
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

	var out outputter
	switch {
	case cmd.detailed:
		return fmt.Errorf("not implemented")
	case cmd.json:
		out = &jsonOutput{
			w: os.Stdout,
		}
	case cmd.dot:
		out = &dotOutput{
			p: p,
			o: cmd.output,
			w: os.Stdout,
		}
	default:
		out = &tableOutput{
			w: tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0),
		}
	}
	return runStatusAll(out, p, sm)
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
}

type MissingStatus struct {
	ProjectRoot     string
	MissingPackages []string
}

func runStatusAll(out outputter, p *dep.Project, sm *gps.SourceMgr) error {
	if p.Lock == nil {
		// TODO if we have no lock file, do...other stuff
		return nil
	}

	// While the network churns on ListVersions() requests, statically analyze
	// code from the current project.
	ptree, err := gps.ListPackages(p.AbsRoot, string(p.ImportRoot))
	if err != nil {
		return fmt.Errorf("analysis of local packages failed: %v", err)
	}

	// Set up a solver in order to check the InputHash.
	params := gps.SolveParameters{
		RootDir:         p.AbsRoot,
		RootPackageTree: ptree,
		Manifest:        p.Manifest,
		// Locks aren't a part of the input hash check, so we can omit it.
	}
	if *verbose {
		params.Trace = true
		params.TraceLogger = log.New(os.Stderr, "", 0)
	}

	s, err := gps.Prepare(params, sm)
	if err != nil {
		return fmt.Errorf("could not set up solver for input hashing: %s", err)
	}

	cm := collectConstraints(ptree, p, sm)

	// Get the project list and sort it so that the printed output users see is
	// deterministically ordered. (This may be superfluous if the lock is always
	// written in alpha order, but it doesn't hurt to double down.)
	slp := p.Lock.Projects()
	sort.Sort(dep.SortedLockedProjects(slp))

	if bytes.Equal(s.HashInputs(), p.Lock.Memo) {
		// If these are equal, we're guaranteed that the lock is a transitively
		// complete picture of all deps. That eliminates the need for at least
		// some checks.

		out.BasicHeader()

		for _, proj := range slp {
			bs := BasicStatus{
				ProjectRoot:  string(proj.Ident().ProjectRoot),
				PackageCount: len(proj.Packages()),
			}

			// Get children only for specific outputers
			// in order to avoid slower status process
			switch out.(type) {
			case *dotOutput:
				r := filepath.Join(p.AbsRoot, "vendor", string(proj.Ident().ProjectRoot))
				ptr, err := gps.ListPackages(r, string(proj.Ident().ProjectRoot))

				if err != nil {
					return fmt.Errorf("analysis of %s package failed: %v", proj.Ident().ProjectRoot, err)
				}

				prm, _ := ptr.ToReachMap(true, false, false, nil)
				bs.Children = prm.Flatten(false)
			}

			// Split apart the version from the lock into its constituent parts
			switch tv := proj.Version().(type) {
			case gps.UnpairedVersion:
				bs.Version = tv
			case gps.Revision:
				bs.Revision = tv
			case gps.PairedVersion:
				bs.Version = tv.Unpair()
				bs.Revision = tv.Underlying()
			}

			// Check if the manifest has an override for this project. If so,
			// set that as the constraint.
			if pp, has := p.Manifest.Ovr[proj.Ident().ProjectRoot]; has && pp.Constraint != nil {
				// TODO note somehow that it's overridden
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
				c, has := p.Manifest.Dependencies[proj.Ident().ProjectRoot]
				if !has {
					c.Constraint = gps.Any()
				}
				// TODO: This constraint is only the constraint imposed by the
				// current project, not by any transitive deps. As a result,
				// transitive project deps will always show "any" here.
				bs.Constraint = c.Constraint

				vl, err := sm.ListVersions(proj.Ident())
				if err == nil {
					gps.SortForUpgrade(vl)

					for _, v := range vl {
						// Because we've sorted the version list for
						// upgrade, the first version we encounter that
						// matches our constraint will be what we want.
						if c.Constraint.Matches(v) {
							// For branch constraints this should be the
							// most recent revision on the selected
							// branch.
							if tv, ok := v.(gps.PairedVersion); ok && v.Type() == gps.IsBranch {
								bs.Latest = tv.Underlying()
							} else {
								bs.Latest = v
							}
							break
						}
					}
				}
			}

			out.BasicLine(&bs)
		}
		out.BasicFooter()

		return nil
	}

	// Hash digest mismatch may indicate that some deps are no longer
	// needed, some are missing, or that some constraints or source
	// locations have changed.
	//
	// It's possible for digests to not match, but still have a correct
	// lock.
	out.MissingHeader()

	rm, _ := ptree.ToReachMap(true, true, false, nil)

	external := rm.Flatten(false)
	roots := make(map[gps.ProjectRoot][]string)
	var errs []string
	for _, e := range external {
		root, err := sm.DeduceProjectRoot(e)
		if err != nil {
			errs = append(errs, string(root))
			continue
		}

		roots[root] = append(roots[root], e)
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

		out.MissingLine(&MissingStatus{ProjectRoot: string(root), MissingPackages: pkgs})
	}
	out.MissingFooter()

	return nil
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

func collectConstraints(ptree gps.PackageTree, p *dep.Project, sm *gps.SourceMgr) map[string][]gps.Constraint {
	// TODO
	return map[string][]gps.Constraint{}
}
