package vsolver

import (
	"container/heap"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"

	"github.com/armon/go-radix"
	"github.com/hashicorp/go-immutable-radix"
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

	// A radix tree representing the immediate externally reachable packages, as
	// determined by static analysis of the root project.
	xt *iradix.Tree
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

	// Initialize queues
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
	pa, err := s.solve()

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
	r.p = make([]LockedProject, len(pa))
	for k, p := range pa {
		r.p[k] = pa2lp(p)
	}

	return r, nil
}

// solve is the top-level loop for the SAT solving process.
func (s *solver) solve() ([]ProjectAtom, error) {
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
		if _, is := s.sel.selected(bmi.id); !is {
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
			// TODO fill in this path - when we're adding more pkgs to an
			// existing, already-selected project
		}
	}

	// Getting this far means we successfully found a solution
	var projs []ProjectAtom
	// Skip the first project - it's always the root, and we don't want to
	// include that in the results.
	for _, p := range s.sel.projects[1:] {
		projs = append(projs, p)
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

	pkgs, err := s.b.listPackages(pa.Ident, nil)
	if err != nil {
		return err
	}

	list := make([]string, len(pkgs))
	k := 0
	for path := range pkgs {
		list[k] = path
		k++
	}

	a := atomWithPackages{
		atom: pa,
		pl:   list,
	}

	// Push the root project onto the queue.
	// TODO maybe it'd just be better to skip this?
	s.sel.projects = append(s.sel.projects, a)

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

	allex, err := s.b.externalReach(a.atom.Ident, a.atom.Version)
	if err != nil {
		return nil, err
	}

	// Use a map to dedupe the unique external packages
	exmap := make(map[string]struct{})
	// Add the packages explicitly listed in the atom to the reach list
	for _, pkg := range a.pl {
		exmap[pkg] = struct{}{}
	}

	// Now, add in the ones we already knew about
	// FIXME this is almost certainly wrong, as it is jumping the gap between
	// projects that have actually been selected, and the imports and
	// constraints expressed by those projects.
	curp := s.sel.getSelectedPackagesIn(a.atom.Ident)
	for pkg := range curp {
		if expkgs, exists := allex[pkg]; !exists {
			// It should be impossible for there to be a selected package
			// that's not in the external reach map; such a condition should
			// have been caught earlier during satisfiability checks. So,
			// explicitly panic here (rather than implicitly when we try to
			// retrieve a nonexistent map entry) as a canary.
			panic("canary - selection contains an atom with pkgs that apparently don't actually exist")
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
	// the trie, just assume that's a correct correspondence.
	// TODO could this be a bad assumption...?
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
		if _, idep, match := xt.LongestPrefix(rp); match { //&& strings.HasPrefix(rp, k) {
			// Valid match found. Put it in the dmap, either creating a new
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
		err := s.satisfiable(atomWithPackages{
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

// getDependenciesOf returns the dependencies of the given ProjectAtom, mediated
// through any overrides dictated by the root project.
//
// If it's the root project, also includes dev dependencies, etc.
func (s *solver) getDependenciesOf(pa ProjectAtom) ([]ProjectDep, error) {
	var deps []ProjectDep

	// If we're looking for root's deps, get it from opts rather than sm
	if s.rm.Name() == pa.Ident.LocalName {
		mdeps := append(s.rm.GetDependencies(), s.rm.GetDevDependencies()...)

		reach, err := s.b.computeRootReach(s.o.Root)
		if err != nil {
			return nil, err
		}

		// Create a radix tree with all the projects we know from the manifest
		// TODO make this smarter if/when non-repo-root dirs can be 'projects'
		xt := radix.New()
		for _, dep := range mdeps {
			xt.Insert(string(dep.Ident.LocalName), dep)
		}

		// Step through the reached packages; if they have [prefix] matches in
		// the trie, just assume that's a correct correspondence.
		// TODO this may be a bad assumption.
		dmap := make(map[ProjectDep]struct{})
		for _, rp := range reach {
			// Look for a match, and ensure it's strictly a parent of the input
			if k, dep, match := xt.LongestPrefix(rp); match && strings.HasPrefix(rp, k) {
				// There's a match; add it to the dep map (thereby avoiding
				// duplicates) and move along
				dmap[dep.(ProjectDep)] = struct{}{}
				continue
			}

			// If it's a stdlib package, skip it.
			// TODO this just hardcodes us to the packages in tip - should we
			// have go version magic here, too?
			if _, exists := stdlib[rp]; exists {
				continue
			}

			// No match. Let the SourceManager try to figure out the root
			root, err := deduceRemoteRepo(rp)
			if err != nil {
				// Nothing we can do if we can't suss out a root
				return nil, err
			}

			// Still no matches; make a new ProjectDep with an open constraint
			dep := ProjectDep{
				Ident: ProjectIdentifier{
					LocalName:   ProjectName(root.Base),
					NetworkName: root.Base,
				},
				Constraint: Any(),
			}
			dmap[dep] = struct{}{}
		}

		// Dump all the deps from the map into the expected return slice
		deps = make([]ProjectDep, len(dmap))
		k := 0
		for dep := range dmap {
			deps[k] = dep
			k++
		}
	} else {
		info, err := s.b.getProjectInfo(pa)
		if err != nil {
			return nil, err
		}

		deps = info.GetDependencies()
		// TODO add overrides here...if we impl the concept (which we should)
	}

	return deps, nil
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
			s.unselectLast()
		}

		// Grab the last versionQueue off the list of queues
		q := s.versions[len(s.versions)-1]

		// another assert that the last in s.sel's ids is == q.current
		atom := s.unselectLast()

		// Advance the queue past the current version, which we know is bad
		// TODO is it feasible to make available the failure reason here?
		if q.advance(nil) == nil && !q.isExhausted() {
			// Search for another acceptable version of this failed dep in its queue
			if s.findValidVersion(q, atom.pl) == nil {
				s.logSolve()

				// Found one! Put it back on the selected queue and stop
				// backtracking
				s.selectAtomWithPackages(atomWithPackages{
					atom: ProjectAtom{
						Ident:   q.id,
						Version: q.current(),
					},
					pl: atom.pl,
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
	iname, jname := s.unsel.sl[i].id, s.unsel.sl[j].id

	if iname.eq(jname) {
		return false
	}

	rname := s.rm.Name()
	// *always* put root project first
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

func (s *solver) fail(i ProjectIdentifier) {
	// skip if the root project
	if s.rm.Name() == i.LocalName {
		return
	}

	// just look for the first (oldest) one; the backtracker will necessarily
	// traverse through and pop off any earlier ones
	for _, vq := range s.versions {
		if vq.id.LocalName == i.LocalName {
			vq.failed = true
			return
		}
	}
}

func (s *solver) selectAtomWithPackages(a atomWithPackages) {
	// TODO so...i guess maybe this is just totally redudant with
	// selectVersion()? ugh. well, at least for now, until we things exercise
	// bimodality
	s.unsel.remove(bimodalIdentifier{
		id: a.atom.Ident,
		pl: a.pl,
	})

	s.sel.projects = append(s.sel.projects, a)

	deps, err := s.getImportsAndConstraintsOf(a)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic(fmt.Sprintf("shouldn't be possible %s", err))
	}

	for _, dep := range deps {
		s.sel.pushDep(Dependency{Depender: a.atom, Dep: dep})
		// Add this dep to the unselected queue if the selection contains only
		// the one bit of information we just pushed in.

		if s.sel.depperCount(dep.Ident) == 1 {
			// ...or if the dep is already selected, and the atom we're
			// selecting imports new packages from the dep that aren't already
			// selected

			// ugh ok so...do we search what's in the pkg deps list, and then
			// push the dep into the unselected queue? or maybe we just change
			// the unseleced queue to dedupe on input? what side effects would
			// that have? would it still be safe to backtrack on that queue?
			s.names[dep.Ident.LocalName] = dep.Ident.netName()
			heap.Push(s.unsel, dep.Ident)
		}
	}
}

//func (s *solver) selectVersion(pa ProjectAtom) {
//s.unsel.remove(pa.Ident)
//s.sel.projects = append(s.sel.projects, pa)

//deps, err := s.getImportsAndConstraintsOf(atomWithPackages{atom: pa})
//if err != nil {
//// if we're choosing a package that has errors getting its deps, there's
//// a bigger problem
//// TODO try to create a test that hits this
//panic(fmt.Sprintf("shouldn't be possible %s", err))
//}

//for _, dep := range deps {
//s.sel.pushDep(Dependency{Depender: pa, Dep: dep})

//// add project to unselected queue if this is the first dep on it -
//// otherwise it's already in there, or been selected
//if s.sel.depperCount(dep.Ident) == 1 {
//s.names[dep.Ident.LocalName] = dep.Ident.netName()
//heap.Push(s.unsel, dep.Ident)
//}
//}
//}

func (s *solver) unselectLast() atomWithPackages {
	var awp atomWithPackages
	awp, s.sel.projects = s.sel.projects[len(s.sel.projects)-1], s.sel.projects[:len(s.sel.projects)-1]
	heap.Push(s.unsel, awp.atom.Ident)

	deps, err := s.getImportsAndConstraintsOf(awp)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic("shouldn't be possible")
	}

	for _, dep := range deps {
		s.sel.popDep(dep.Ident)

		// if no parents/importers, remove from unselected queue
		if s.sel.depperCount(dep.Ident) == 0 {
			delete(s.names, dep.Ident.LocalName)
			s.unsel.remove(bimodalIdentifier{id: dep.Ident, pl: dep.pl})
		}
	}

	return awp
}

func (s *solver) logStart(id ProjectIdentifier) {
	if !s.o.Trace {
		return
	}

	prefix := strings.Repeat("| ", len(s.versions)+1)
	s.tl.Printf("%s\n", tracePrefix(fmt.Sprintf("? attempting %s", id.errString()), prefix, prefix))
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
func pa2lp(pa ProjectAtom) LockedProject {
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

	return lp
}
