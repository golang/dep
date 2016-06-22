package vsolver

import (
	"container/heap"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/armon/go-radix"
)

var (
	// With a random revision and no name, collisions are unlikely
	nilpa = ProjectAtom{
		Version: Revision(strconv.FormatInt(rand.Int63(), 36)),
	}
)

// SolveOpts holds options that govern solving behavior, and the proper inputs
// to the solving process.
type SolveOpts struct {
	// The path to the root of the project on which the solver is working.
	Root string

	// The 'name' of the project. Required. This should (must?) correspond to subpath of
	// Root that exists under a GOPATH.
	N ProjectName

	// The root manifest. Required. This contains all the dependencies, constraints, and
	// other controls available to the root project.
	M Manifest

	// The root lock. Optional. Generally, this lock is the output of a previous solve run.
	//
	// If provided, the solver will attempt to preserve the versions specified
	// in the lock, unless ToChange or ChangeAll settings indicate otherwise.
	L Lock

	// Downgrade indicates whether the solver will attempt to upgrade (false) or
	// downgrade (true) projects that are not locked, or are marked for change.
	//
	// Upgrading is, by far, the most typical case. The field is named
	// 'Downgrade' so that the bool's zero value corresponds to that most
	// typical case.
	Downgrade bool

	// ChangeAll indicates that all projects should be changed - that is, any
	// versions specified in the root lock file should be ignored.
	ChangeAll bool

	// ToChange is a list of project names that should be changed - that is, any
	// versions specified for those projects in the root lock file should be
	// ignored.
	//
	// Passing ChangeAll has subtly different behavior from enumerating all
	// projects into ToChange. In general, ToChange should *only* be used if the
	// user expressly requested an upgrade for a specific project.
	ToChange []ProjectName

	// Trace controls whether the solver will generate informative trace output
	// as it moves through the solving process.
	Trace bool

	// TraceLogger is the logger to use for generating trace output. If Trace is
	// true but no logger is provided, solving will result in an error.
	TraceLogger *log.Logger
}

// solver is a CDCL-style SAT solver with satisfiability conditions hardcoded to
// the needs of the Go package management problem space.
type solver struct {
	// The current number of attempts made over the course of this solve. This
	// number increments each time the algorithm completes a backtrack and
	// starts moving forward again.
	attempts int

	// SolveOpts are the configuration options provided to the solver. The
	// solver will abort early if certain options are not appropriately set.
	o SolveOpts

	// Logger used exclusively for trace output, if the trace option is set.
	tl *log.Logger

	// A bridge to the standard SourceManager. The adapter does some local
	// caching of pre-sorted version lists, as well as translation between the
	// full-on ProjectIdentifiers that the solver deals with and the simplified
	// names a SourceManager operates on.
	b sourceBridge

	// The list of projects currently "selected" - that is, they have passed all
	// satisfiability checks, and are part of the current solution.
	//
	// The *selection type is mostly just a dumb data container; the solver
	// itself is responsible for maintaining that invariant.
	sel *selection

	// The current list of projects that we need to incorporate into the solution in
	// order for the solution to be complete. This list is implemented as a
	// priority queue that places projects least likely to induce errors at the
	// front, in order to minimize the amount of backtracking required to find a
	// solution.
	//
	// Entries are added to and removed from this list by the solver at the same
	// time that the selected queue is updated, either with an addition or
	// removal.
	unsel *unselected

	// A list of all the currently active versionQueues in the solver. The set
	// of projects represented here corresponds closely to what's in s.sel,
	// although s.sel will always contain the root project, and s.versions never
	// will.
	versions []*versionQueue // TODO rename to pvq

	// A map of the ProjectName (local names) that should be allowed to change
	chng map[ProjectName]struct{}

	// A map of the ProjectName (local names) that are currently selected, and
	// the network name to which they currently correspond.
	names map[ProjectName]string

	// A map of the names listed in the root's lock.
	rlm map[ProjectIdentifier]LockedProject

	// A normalized, copied version of the root manifest.
	rm Manifest
}

// Solve attempts to find a dependency solution for the given project, as
// represented by the provided SolveOpts.
//
// This is the entry point to the main vsolver workhorse.
func Solve(o SolveOpts, sm SourceManager) (Result, error) {
	s, err := prepareSolver(o, sm)
	if err != nil {
		return nil, err
	}

	return s.run()
}

// prepare reads from the SolveOpts and prepare the solver to run.
func prepareSolver(opts SolveOpts, sm SourceManager) (*solver, error) {
	// local overrides would need to be handled first.
	// TODO local overrides! heh

	if opts.M == nil {
		return nil, BadOptsFailure("Opts must include a manifest.")
	}
	if opts.Root == "" {
		return nil, BadOptsFailure("Opts must specify a non-empty string for the project root directory.")
	}
	if opts.N == "" {
		return nil, BadOptsFailure("Opts must include a project name.")
	}
	if opts.Trace && opts.TraceLogger == nil {
		return nil, BadOptsFailure("Trace requested, but no logger provided.")
	}

	s := &solver{
		o:  opts,
		b:  newBridge(opts.N, opts.Root, sm, opts.Downgrade),
		tl: opts.TraceLogger,
	}

	// Initialize maps
	s.chng = make(map[ProjectName]struct{})
	s.rlm = make(map[ProjectIdentifier]LockedProject)
	s.names = make(map[ProjectName]string)

	// Initialize stacks and queues
	s.sel = &selection{
		deps: make(map[ProjectIdentifier][]Dependency),
		sm:   s.b,
	}
	s.unsel = &unselected{
		sl:  make([]bimodalIdentifier, 0),
		cmp: s.unselectedComparator,
	}

	return s, nil
}

// run executes the solver and creates an appropriate result.
func (s *solver) run() (Result, error) {
	// Ensure the root is in good, working order before doing anything else
	err := s.b.verifyRoot(s.o.Root)
	if err != nil {
		return nil, err
	}

	// Prep safe, normalized versions of root manifest and lock data
	s.rm = prepManifest(s.o.M)

	if s.o.L != nil {
		for _, lp := range s.o.L.Projects() {
			s.rlm[lp.Ident().normalize()] = lp
		}
	}

	for _, v := range s.o.ToChange {
		s.chng[v] = struct{}{}
	}

	// Prime the queues with the root project
	err = s.selectRoot()
	if err != nil {
		// TODO this properly with errs, yar
		panic("couldn't select root, yikes")
	}

	// Log initial step
	s.logSolve()
	all, err := s.solve()

	// Solver finished with an err; return that and we're done
	if err != nil {
		return nil, err
	}

	// Solved successfully, create and return a result
	r := result{
		att: s.attempts,
		hd:  s.o.HashInputs(),
	}

	// Convert ProjectAtoms into LockedProjects
	r.p = make([]LockedProject, len(all))
	k := 0
	for pa, pl := range all {
		r.p[k] = pa2lp(pa, pl)
		k++
	}

	return r, nil
}

// solve is the top-level loop for the SAT solving process.
func (s *solver) solve() (map[ProjectAtom]map[string]struct{}, error) {
	// Main solving loop
	for {
		bmi, has := s.nextUnselected()

		if !has {
			// no more packages to select - we're done.
			break
		}

		// This split is the heart of "bimodal solving": we follow different
		// satisfiability and selection paths depending on whether we've already
		// selected the base project/repo that came off the unselected queue.
		//
		// (If we already have selected the project, other parts of the
		// algorithm guarantee the bmi will contain at least one package from
		// this project that has yet to be selected.)
		if awp, is := s.sel.selected(bmi.id); !is {
			// Analysis path for when we haven't selected the project yet - need
			// to create a version queue.
			s.logStart(bmi)
			queue, err := s.createVersionQueue(bmi)
			if err != nil {
				// Err means a failure somewhere down the line; try backtracking.
				if s.backtrack() {
					// backtracking succeeded, move to the next unselected id
					continue
				}
				return nil, err
			}

			if queue.current() == nil {
				panic("canary - queue is empty, but flow indicates success")
			}

			s.selectAtomWithPackages(atomWithPackages{
				atom: ProjectAtom{
					Ident:   queue.id,
					Version: queue.current(),
				},
				pl: bmi.pl,
			})
			s.versions = append(s.versions, queue)
			s.logSolve()
		} else {
			// We're just trying to add packages to an already-selected project.
			// That means it's not OK to burn through the version queue for that
			// project as we do when first selecting a project, as doing so
			// would upend the guarantees on which all previous selections of
			// the project are based (both the initial one, and any package-only
			// ones).

			// Because we can only safely operate within the scope of the
			// single, currently selected version, we can skip looking for the
			// queue and just use the version given in what came back from
			// s.sel.selected().
			nawp := atomWithPackages{
				atom: ProjectAtom{
					Ident:   bmi.id,
					Version: awp.atom.Version,
				},
				pl: bmi.pl,
			}

			s.logStart(bmi) // TODO different special start logger for this path
			err := s.checkPackage(nawp)
			if err != nil {
				// Err means a failure somewhere down the line; try backtracking.
				if s.backtrack() {
					// backtracking succeeded, move to the next unselected id
					continue
				}
				return nil, err
			}
			s.selectPackages(nawp)
			// We don't add anything to the stack of version queues because the
			// backtracker knows not to popping the vqstack if it backtracks
			// across a package addition.
			s.logSolve()
		}
	}

	// Getting this far means we successfully found a solution. Combine the
	// selected projects and packages.
	projs := make(map[ProjectAtom]map[string]struct{})

	// Skip the first project. It's always the root, and that shouldn't be
	// included in results.
	for _, sel := range s.sel.projects[1:] {
		pm, exists := projs[sel.a.atom]
		if !exists {
			pm = make(map[string]struct{})
			projs[sel.a.atom] = pm
		}

		for _, path := range sel.a.pl {
			pm[path] = struct{}{}
		}
	}
	return projs, nil
}

// selectRoot is a specialized selectAtomWithPackages, used solely to initially
// populate the queues at the beginning of a solve run.
func (s *solver) selectRoot() error {
	pa := ProjectAtom{
		Ident: ProjectIdentifier{
			LocalName: s.o.N,
		},
		// This is a hack so that the root project doesn't have a nil version.
		// It's sort of OK because the root never makes it out into the results.
		// We may need a more elegant solution if we discover other side
		// effects, though.
		Version: Revision(""),
	}

	ptree, err := s.b.listPackages(pa.Ident, nil)
	if err != nil {
		return err
	}

	list := make([]string, len(ptree.Packages))
	k := 0
	for path := range ptree.Packages {
		list[k] = path
		k++
	}

	a := atomWithPackages{
		atom: pa,
		pl:   list,
	}

	// Push the root project onto the queue.
	// TODO maybe it'd just be better to skip this?
	s.sel.pushSelection(a, true)

	// If we're looking for root's deps, get it from opts and local root
	// analysis, rather than having the sm do it
	mdeps := append(s.rm.GetDependencies(), s.rm.GetDevDependencies()...)
	reach, err := s.b.computeRootReach(s.o.Root)
	if err != nil {
		return err
	}

	deps, err := s.intersectConstraintsWithImports(mdeps, reach)
	if err != nil {
		// TODO this could well happen; handle it with a more graceful error
		panic(fmt.Sprintf("shouldn't be possible %s", err))
	}

	for _, dep := range deps {
		s.sel.pushDep(Dependency{Depender: pa, Dep: dep})
		// Add all to unselected queue
		s.names[dep.Ident.LocalName] = dep.Ident.netName()
		heap.Push(s.unsel, bimodalIdentifier{id: dep.Ident, pl: dep.pl})
	}

	return nil
}

func (s *solver) getImportsAndConstraintsOf(a atomWithPackages) ([]completeDep, error) {
	var err error

	if s.rm.Name() == a.atom.Ident.LocalName {
		panic("Should never need to recheck imports/constraints from root during solve")
	}

	// Work through the source manager to get project info and static analysis
	// information.
	info, err := s.b.getProjectInfo(a.atom)
	if err != nil {
		return nil, err
	}

	ptree, err := s.b.listPackages(a.atom.Ident, a.atom.Version)
	if err != nil {
		return nil, err
	}

	allex, err := ptree.ExternalReach(false, false)
	if err != nil {
		return nil, err
	}

	// Use a map to dedupe the unique external packages
	exmap := make(map[string]struct{})
	// Add the packages reached by the packages explicitly listed in the atom to
	// the list
	for _, pkg := range a.pl {
		if expkgs, exists := allex[pkg]; !exists {
			return nil, fmt.Errorf("Package %s does not exist within project %s", pkg, a.atom.Ident.errString())
		} else {
			for _, ex := range expkgs {
				exmap[ex] = struct{}{}
			}
		}
	}

	reach := make([]string, len(exmap))
	k := 0
	for pkg := range exmap {
		reach[k] = pkg
		k++
	}

	deps := info.GetDependencies()
	// TODO add overrides here...if we impl the concept (which we should)

	return s.intersectConstraintsWithImports(deps, reach)
}

// intersectConstraintsWithImports takes a list of constraints and a list of
// externally reached packages, and creates a []completeDep that is guaranteed
// to include all packages named by import reach, using constraints where they
// are available, or Any() where they are not.
func (s *solver) intersectConstraintsWithImports(deps []ProjectDep, reach []string) ([]completeDep, error) {
	// Create a radix tree with all the projects we know from the manifest
	// TODO make this smarter once we allow non-root inputs as 'projects'
	xt := radix.New()
	for _, dep := range deps {
		xt.Insert(string(dep.Ident.LocalName), dep)
	}

	// Step through the reached packages; if they have prefix matches in
	// the trie, assume (mostly) it's a correct correspondence.
	dmap := make(map[ProjectName]completeDep)
	for _, rp := range reach {
		// If it's a stdlib package, skip it.
		// TODO this just hardcodes us to the packages in tip - should we
		// have go version magic here, too?
		if _, exists := stdlib[rp]; exists {
			continue
		}

		// Look for a prefix match; it'll be the root project/repo containing
		// the reached package
		if k, idep, match := xt.LongestPrefix(rp); match {
			// The radix tree gets it mostly right, but we have to guard against
			// possibilities like this:
			//
			// github.com/sdboyer/foo
			// github.com/sdboyer/foobar/baz
			//
			// The latter would incorrectly be conflated in with the former. So,
			// as we know we're operating on strings that describe paths, guard
			// against this case by verifying that either the input is the same
			// length as the match (in which case we know they're equal), or
			// that the next character is the is the PathSeparator.
			if len(k) == len(rp) || strings.IndexRune(rp[:len(k)], os.PathSeparator) == 0 {
				// Match is valid; put it in the dmap, either creating a new
				// completeDep or appending it to the existing one for this base
				// project/prefix.
				dep := idep.(ProjectDep)
				if cdep, exists := dmap[dep.Ident.LocalName]; exists {
					cdep.pl = append(cdep.pl, rp)
					dmap[dep.Ident.LocalName] = cdep
				} else {
					dmap[dep.Ident.LocalName] = completeDep{
						ProjectDep: dep,
						pl:         []string{rp},
					}
				}
				continue
			}
		}

		// No match. Let the SourceManager try to figure out the root
		root, err := s.b.deduceRemoteRepo(rp)
		if err != nil {
			// Nothing we can do if we can't suss out a root
			return nil, err
		}

		// Still no matches; make a new completeDep with an open constraint
		pd := ProjectDep{
			Ident: ProjectIdentifier{
				LocalName:   ProjectName(root.Base),
				NetworkName: root.Base,
			},
			Constraint: Any(),
		}

		// Insert the pd into the trie so that further deps from this
		// project get caught by the prefix search
		xt.Insert(root.Base, pd)
		// And also put the complete dep into the dmap
		dmap[ProjectName(root.Base)] = completeDep{
			ProjectDep: pd,
			pl:         []string{rp},
		}
	}

	// Dump all the deps from the map into the expected return slice
	cdeps := make([]completeDep, len(dmap))
	k := 0
	for _, cdep := range dmap {
		cdeps[k] = cdep
		k++
	}

	return cdeps, nil
}

func (s *solver) createVersionQueue(bmi bimodalIdentifier) (*versionQueue, error) {
	id := bmi.id
	// If on the root package, there's no queue to make
	if id.LocalName == s.rm.Name() {
		return newVersionQueue(id, nilpa, s.b)
	}

	exists, err := s.b.repoExists(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		exists, err = s.b.vendorCodeExists(id)
		if err != nil {
			return nil, err
		}
		if exists {
			// Project exists only in vendor (and in some manifest somewhere)
			// TODO mark this for special handling, somehow?
		} else {
			return nil, newSolveError(fmt.Sprintf("Project '%s' could not be located.", id), cannotResolve)
		}
	}

	lockv := nilpa
	if len(s.rlm) > 0 {
		lockv, err = s.getLockVersionIfValid(id)
		if err != nil {
			// Can only get an error here if an upgrade was expressly requested on
			// code that exists only in vendor
			return nil, err
		}
	}

	q, err := newVersionQueue(id, lockv, s.b)
	if err != nil {
		// TODO this particular err case needs to be improved to be ONLY for cases
		// where there's absolutely nothing findable about a given project name
		return nil, err
	}

	return q, s.findValidVersion(q, bmi.pl)
}

// findValidVersion walks through a versionQueue until it finds a version that
// satisfies the constraints held in the current state of the solver.
//
// The satisfiability checks triggered from here are constrained to operate only
// on those dependencies induced by the list of packages given in the second
// parameter.
func (s *solver) findValidVersion(q *versionQueue, pl []string) error {
	if nil == q.current() {
		// TODO this case shouldn't be reachable, but panic here as a canary
		panic("version queue is empty, should not happen")
	}

	faillen := len(q.fails)

	for {
		cur := q.current()
		err := s.checkProject(atomWithPackages{
			atom: ProjectAtom{
				Ident:   q.id,
				Version: cur,
			},
			pl: pl,
		})
		if err == nil {
			// we have a good version, can return safely
			return nil
		}

		if q.advance(err) != nil {
			// Error on advance, have to bail out
			break
		}
		if q.isExhausted() {
			// Queue is empty, bail with error
			break
		}
	}

	s.fail(s.sel.getDependenciesOn(q.id)[0].Depender.Ident)

	// Return a compound error of all the new errors encountered during this
	// attempt to find a new, valid version
	return &noVersionError{
		pn:    q.id,
		fails: q.fails[faillen:],
	}
}

// getLockVersionIfValid finds an atom for the given ProjectIdentifier from the
// root lock, assuming:
//
// 1. A root lock was provided
// 2. The general flag to change all projects was not passed
// 3. A flag to change this particular ProjectIdentifier was not passed
//
// If any of these three conditions are true (or if the id cannot be found in
// the root lock), then no atom will be returned.
func (s *solver) getLockVersionIfValid(id ProjectIdentifier) (ProjectAtom, error) {
	// If the project is specifically marked for changes, then don't look for a
	// locked version.
	if _, explicit := s.chng[id.LocalName]; explicit || s.o.ChangeAll {
		// For projects with an upstream or cache repository, it's safe to
		// ignore what's in the lock, because there's presumably more versions
		// to be found and attempted in the repository. If it's only in vendor,
		// though, then we have to try to use what's in the lock, because that's
		// the only version we'll be able to get.
		if exist, _ := s.b.repoExists(id); exist {
			return nilpa, nil
		}

		// However, if a change was *expressly* requested for something that
		// exists only in vendor, then that guarantees we don't have enough
		// information to complete a solution. In that case, error out.
		if explicit {
			return nilpa, &missingSourceFailure{
				goal: id,
				prob: "Cannot upgrade %s, as no source repository could be found.",
			}
		}
	}

	lp, exists := s.rlm[id]
	if !exists {
		return nilpa, nil
	}

	constraint := s.sel.getConstraint(id)
	v := lp.Version()
	if !constraint.Matches(v) {
		var found bool
		if tv, ok := v.(Revision); ok {
			// If we only have a revision from the root's lock, allow matching
			// against other versions that have that revision
			for _, pv := range s.b.pairRevision(id, tv) {
				if constraint.Matches(pv) {
					v = pv
					found = true
					break
				}
			}
			//} else if _, ok := constraint.(Revision); ok {
			//// If the current constraint is itself a revision, and the lock gave
			//// an unpaired version, see if they match up
			////
			//if u, ok := v.(UnpairedVersion); ok {
			//pv := s.sm.pairVersion(id, u)
			//if constraint.Matches(pv) {
			//v = pv
			//found = true
			//}
			//}
		}

		if !found {
			s.logSolve("%s in root lock, but current constraints disallow it", id.errString())
			return nilpa, nil
		}
	}

	s.logSolve("using root lock's version of %s", id.errString())

	return ProjectAtom{
		Ident:   id,
		Version: v,
	}, nil
}

// backtrack works backwards from the current failed solution to find the next
// solution to try.
func (s *solver) backtrack() bool {
	if len(s.versions) == 0 {
		// nothing to backtrack to
		return false
	}

	for {
		for {
			if len(s.versions) == 0 {
				// no more versions, nowhere further to backtrack
				return false
			}
			if s.versions[len(s.versions)-1].failed {
				break
			}

			s.versions, s.versions[len(s.versions)-1] = s.versions[:len(s.versions)-1], nil

			// Pop selections off until we get to a project.
			var proj bool
			for !proj {
				_, proj = s.unselectLast()
			}
		}

		// Grab the last versionQueue off the list of queues
		q := s.versions[len(s.versions)-1]
		// Walk back to the next project
		var awp atomWithPackages
		var proj bool

		for !proj {
			awp, proj = s.unselectLast()
		}

		if !q.id.eq(awp.atom.Ident) {
			panic("canary - version queue stack and selected project stack are out of alignment")
		}

		// Advance the queue past the current version, which we know is bad
		// TODO is it feasible to make available the failure reason here?
		if q.advance(nil) == nil && !q.isExhausted() {
			// Search for another acceptable version of this failed dep in its queue
			if s.findValidVersion(q, awp.pl) == nil {
				s.logSolve()

				// Found one! Put it back on the selected queue and stop
				// backtracking
				s.selectAtomWithPackages(atomWithPackages{
					atom: ProjectAtom{
						Ident:   q.id,
						Version: q.current(),
					},
					pl: awp.pl,
				})
				break
			}
		}

		s.logSolve("no more versions of %s, backtracking", q.id.errString())

		// No solution found; continue backtracking after popping the queue
		// we just inspected off the list
		// GC-friendly pop pointer elem in slice
		s.versions, s.versions[len(s.versions)-1] = s.versions[:len(s.versions)-1], nil
	}

	// Backtracking was successful if loop ended before running out of versions
	if len(s.versions) == 0 {
		return false
	}
	s.attempts++
	return true
}

func (s *solver) nextUnselected() (bimodalIdentifier, bool) {
	if len(s.unsel.sl) > 0 {
		return s.unsel.sl[0], true
	}

	return bimodalIdentifier{}, false
}

func (s *solver) unselectedComparator(i, j int) bool {
	ibmi, jbmi := s.unsel.sl[i], s.unsel.sl[j]
	iname, jname := ibmi.id, jbmi.id

	// Most important thing is pushing package additions ahead of project
	// additions. Package additions can't walk their version queue, so all they
	// do is narrow the possibility of success; better to find out early and
	// fast if they're going to fail than wait until after we've done real work
	// on a project and have to backtrack across it.

	// FIXME the impl here is currently O(n) in the number of selections; it
	// absolutely cannot stay in a hot sorting path like this
	_, isel := s.sel.selected(iname)
	_, jsel := s.sel.selected(jname)

	if isel && !jsel {
		return true
	}
	if !isel && jsel {
		return false
	}

	if iname.eq(jname) {
		return false
	}

	rname := s.rm.Name()
	// *always* put root project first
	// TODO wait, it shouldn't be possible to have root in here...?
	if iname.LocalName == rname {
		return true
	}
	if jname.LocalName == rname {
		return false
	}

	_, ilock := s.rlm[iname]
	_, jlock := s.rlm[jname]

	switch {
	case ilock && !jlock:
		return true
	case !ilock && jlock:
		return false
	case ilock && jlock:
		return iname.less(jname)
	}

	// Now, sort by number of available versions. This will trigger network
	// activity, but at this point we know that the project we're looking at
	// isn't locked by the root. And, because being locked by root is the only
	// way avoid that call when making a version queue, we know we're gonna have
	// to pay that cost anyway.
	//
	// TODO ...at least, 'til we allow 'preferred' versions via non-root locks

	// We can safely ignore an err from ListVersions here because, if there is
	// an actual problem, it'll be noted and handled somewhere else saner in the
	// solving algorithm.
	ivl, _ := s.b.listVersions(iname)
	jvl, _ := s.b.listVersions(jname)
	iv, jv := len(ivl), len(jvl)

	// Packages with fewer versions to pick from are less likely to benefit from
	// backtracking, so deal with them earlier in order to minimize the amount
	// of superfluous backtracking through them we do.
	switch {
	case iv == 0 && jv != 0:
		return true
	case iv != 0 && jv == 0:
		return false
	case iv != jv:
		return iv < jv
	}

	// Finally, if all else fails, fall back to comparing by name
	return iname.less(jname)
}

func (s *solver) fail(id ProjectIdentifier) {
	// TODO does this need updating, now that we have non-project package
	// selection?

	// skip if the root project
	if s.rm.Name() != id.LocalName {
		// just look for the first (oldest) one; the backtracker will necessarily
		// traverse through and pop off any earlier ones
		for _, vq := range s.versions {
			if vq.id.eq(id) {
				vq.failed = true
				return
			}
		}
	}
}

// selectAtomWithPackages handles the selection case where a new project is
// being added to the selection queue, alongside some number of its contained
// packages. This method pushes them onto the selection queue, then adds any
// new resultant deps to the unselected queue.
func (s *solver) selectAtomWithPackages(a atomWithPackages) {
	s.unsel.remove(bimodalIdentifier{
		id: a.atom.Ident,
		pl: a.pl,
	})

	s.sel.pushSelection(a, true)

	deps, err := s.getImportsAndConstraintsOf(a)
	if err != nil {
		// This shouldn't be possible; other checks should have ensured all
		// packages and deps are present for any argument passed to this method.
		panic(fmt.Sprintf("canary - shouldn't be possible %s", err))
	}

	for _, dep := range deps {
		s.sel.pushDep(Dependency{Depender: a.atom, Dep: dep})
		// Go through all the packages introduced on this dep, selecting only
		// the ones where the only depper on them is what we pushed in. Then,
		// put those into the unselected queue.
		rpm := s.sel.getRequiredPackagesIn(dep.Ident)
		var newp []string
		for _, pkg := range dep.pl {
			if rpm[pkg] == 1 {
				newp = append(newp, pkg)
			}
		}

		if len(newp) > 0 {
			heap.Push(s.unsel, bimodalIdentifier{id: dep.Ident, pl: newp})
		}

		if s.sel.depperCount(dep.Ident) == 1 {
			s.names[dep.Ident.LocalName] = dep.Ident.netName()
		}
	}
}

// selectPackages handles the selection case where we're just adding some new
// packages to a project that was already selected. After pushing the selection,
// it adds any newly-discovered deps to the unselected queue.
//
// It also takes an atomWithPackages because we need that same information in
// order to enqueue the selection.
func (s *solver) selectPackages(a atomWithPackages) {
	s.unsel.remove(bimodalIdentifier{
		id: a.atom.Ident,
		pl: a.pl,
	})

	s.sel.pushSelection(a, false)

	deps, err := s.getImportsAndConstraintsOf(a)
	if err != nil {
		// This shouldn't be possible; other checks should have ensured all
		// packages and deps are present for any argument passed to this method.
		panic(fmt.Sprintf("canary - shouldn't be possible %s", err))
	}

	for _, dep := range deps {
		s.sel.pushDep(Dependency{Depender: a.atom, Dep: dep})
		// Go through all the packages introduced on this dep, selecting only
		// the ones where the only depper on them is what we pushed in. Then,
		// put those into the unselected queue.
		rpm := s.sel.getRequiredPackagesIn(dep.Ident)
		var newp []string
		for _, pkg := range dep.pl {
			if rpm[pkg] == 1 {
				newp = append(newp, pkg)
			}
		}

		if len(newp) > 0 {
			heap.Push(s.unsel, bimodalIdentifier{id: dep.Ident, pl: newp})
		}

		if s.sel.depperCount(dep.Ident) == 1 {
			s.names[dep.Ident.LocalName] = dep.Ident.netName()
		}
	}
}

func (s *solver) unselectLast() (atomWithPackages, bool) {
	awp, first := s.sel.popSelection()
	heap.Push(s.unsel, bimodalIdentifier{id: awp.atom.Ident, pl: awp.pl})

	deps, err := s.getImportsAndConstraintsOf(awp)
	if err != nil {
		// This shouldn't be possible; other checks should have ensured all
		// packages and deps are present for any argument passed to this method.
		panic(fmt.Sprintf("canary - shouldn't be possible %s", err))
	}

	for _, dep := range deps {
		s.sel.popDep(dep.Ident)

		// if no parents/importers, remove from unselected queue
		if s.sel.depperCount(dep.Ident) == 0 {
			delete(s.names, dep.Ident.LocalName)
			s.unsel.remove(bimodalIdentifier{id: dep.Ident, pl: dep.pl})
		}
	}

	return awp, first
}

func (s *solver) logStart(bmi bimodalIdentifier) {
	if !s.o.Trace {
		return
	}

	prefix := strings.Repeat("| ", len(s.versions)+1)
	// TODO how...to list the packages in the limited space we have?
	s.tl.Printf("%s\n", tracePrefix(fmt.Sprintf("? attempting %s (with %v packages)", bmi.id.errString(), len(bmi.pl)), prefix, prefix))
}

func (s *solver) logSolve(args ...interface{}) {
	if !s.o.Trace {
		return
	}

	preflen := len(s.versions)
	var msg string
	if len(args) == 0 {
		// Generate message based on current solver state
		if len(s.versions) == 0 {
			msg = "✓ (root)"
		} else {
			vq := s.versions[len(s.versions)-1]
			msg = fmt.Sprintf("✓ select %s at %s", vq.id.errString(), vq.current())
		}
	} else {
		// Use longer prefix length for these cases, as they're the intermediate
		// work
		preflen++
		switch data := args[0].(type) {
		case string:
			msg = tracePrefix(fmt.Sprintf(data, args[1:]), "| ", "| ")
		case traceError:
			// We got a special traceError, use its custom method
			msg = tracePrefix(data.traceString(), "| ", "x ")
		case error:
			// Regular error; still use the x leader but default Error() string
			msg = tracePrefix(data.Error(), "| ", "x ")
		default:
			// panic here because this can *only* mean a stupid internal bug
			panic("canary - must pass a string as first arg to logSolve, or no args at all")
		}
	}

	prefix := strings.Repeat("| ", preflen)
	s.tl.Printf("%s\n", tracePrefix(msg, prefix, prefix))
}

func tracePrefix(msg, sep, fsep string) string {
	parts := strings.Split(strings.TrimSuffix(msg, "\n"), "\n")
	for k, str := range parts {
		if k == 0 {
			parts[k] = fmt.Sprintf("%s%s", fsep, str)
		} else {
			parts[k] = fmt.Sprintf("%s%s", sep, str)
		}
	}

	return strings.Join(parts, "\n")
}

// simple (temporary?) helper just to convert atoms into locked projects
func pa2lp(pa ProjectAtom, pkgs map[string]struct{}) LockedProject {
	lp := LockedProject{
		pi: pa.Ident.normalize(), // shouldn't be necessary, but normalize just in case
		// path is unnecessary duplicate information now, but if we ever allow
		// nesting as a conflict resolution mechanism, it will become valuable
		path: string(pa.Ident.LocalName),
	}

	switch v := pa.Version.(type) {
	case UnpairedVersion:
		lp.v = v
	case Revision:
		lp.r = v
	case versionPair:
		lp.v = v.v
		lp.r = v.r
	default:
		panic("unreachable")
	}

	for pkg := range pkgs {
		lp.pkgs = append(lp.pkgs, strings.TrimPrefix(pkg, string(pa.Ident.LocalName)+string(os.PathSeparator)))
	}
	sort.Strings(lp.pkgs)

	return lp
}
