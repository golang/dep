package gps

import (
	"container/heap"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/armon/go-radix"
)

var rootRev = Revision("")

// SolveParameters hold all arguments to a solver run.
//
// Only RootDir and ImportRoot are absolutely required. A nil Manifest is
// allowed, though it usually makes little sense.
//
// Of these properties, only Manifest and Ignore are (directly) incorporated in
// memoization hashing.
type SolveParameters struct {
	// The path to the root of the project on which the solver should operate.
	// This should point to the directory that should contain the vendor/
	// directory.
	//
	// In general, it is wise for this to be under an active GOPATH, though it
	// is not (currently) required.
	//
	// A real path to a readable directory is required.
	RootDir string

	// The import path at the base of all import paths covered by the project.
	// For example, the appropriate value for gps itself here is:
	//
	//  github.com/sdboyer/gps
	//
	// In most cases, this should match the latter portion of RootDir. However,
	// that is not (currently) required.
	//
	// A non-empty string is required.
	ImportRoot ProjectRoot

	// The root manifest. This contains all the dependency constraints
	// associated with normal Manifests, as well as the particular controls
	// afforded only to the root project.
	//
	// May be nil, but for most cases, that would be unwise.
	Manifest RootManifest

	// The root lock. Optional. Generally, this lock is the output of a previous
	// solve run.
	//
	// If provided, the solver will attempt to preserve the versions specified
	// in the lock, unless ToChange or ChangeAll settings indicate otherwise.
	Lock Lock

	// ToChange is a list of project names that should be changed - that is, any
	// versions specified for those projects in the root lock file should be
	// ignored.
	//
	// Passing ChangeAll has subtly different behavior from enumerating all
	// projects into ToChange. In general, ToChange should *only* be used if the
	// user expressly requested an upgrade for a specific project.
	ToChange []ProjectRoot

	// ChangeAll indicates that all projects should be changed - that is, any
	// versions specified in the root lock file should be ignored.
	ChangeAll bool

	// Downgrade indicates whether the solver will attempt to upgrade (false) or
	// downgrade (true) projects that are not locked, or are marked for change.
	//
	// Upgrading is, by far, the most typical case. The field is named
	// 'Downgrade' so that the bool's zero value corresponds to that most
	// typical case.
	Downgrade bool

	// Trace controls whether the solver will generate informative trace output
	// as it moves through the solving process.
	Trace bool

	// TraceLogger is the logger to use for generating trace output. If Trace is
	// true but no logger is provided, solving will result in an error.
	TraceLogger *log.Logger
}

// solver is a CDCL-style constraint solver with satisfiability conditions
// hardcoded to the needs of the Go package management problem space.
type solver struct {
	// The current number of attempts made over the course of this solve. This
	// number increments each time the algorithm completes a backtrack and
	// starts moving forward again.
	attempts int

	// SolveParameters are the inputs to the solver. They determine both what
	// data the solver should operate on, and certain aspects of how solving
	// proceeds.
	//
	// Prepare() validates these, so by the time we have a *solver instance, we
	// know they're valid.
	params SolveParameters

	// Logger used exclusively for trace output, if the trace option is set.
	tl *log.Logger

	// A bridge to the standard SourceManager. The adapter does some local
	// caching of pre-sorted version lists, as well as translation between the
	// full-on ProjectIdentifiers that the solver deals with and the simplified
	// names a SourceManager operates on.
	b sourceBridge

	// A stack containing projects and packages that are currently "selected" -
	// that is, they have passed all satisfiability checks, and are part of the
	// current solution.
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

	// Map of packages to ignore. Derived by converting SolveParameters.Ignore
	// into a map during solver prep - which also, nicely, deduplicates it.
	ig map[string]bool

	// A stack of all the currently active versionQueues in the solver. The set
	// of projects represented here corresponds closely to what's in s.sel,
	// although s.sel will always contain the root project, and s.vqs never
	// will. Also, s.vqs is only added to (or popped from during backtracking)
	// when a new project is selected; it is untouched when new packages are
	// added to an existing project.
	vqs []*versionQueue

	// A map of the ProjectRoot (local names) that should be allowed to change
	chng map[ProjectRoot]struct{}

	// A map of the ProjectRoot (local names) that are currently selected, and
	// the network name to which they currently correspond.
	// TODO(sdboyer) i think this is cruft and can be removed
	names map[ProjectRoot]string

	// A ProjectConstraints map containing the validated (guaranteed non-empty)
	// overrides declared by the root manifest.
	ovr ProjectConstraints

	// A map of the names listed in the root's lock.
	rlm map[ProjectIdentifier]LockedProject

	// A normalized, copied version of the root manifest.
	rm Manifest

	// A normalized, copied version of the root lock.
	rl Lock
}

// A Solver is the main workhorse of gps: given a set of project inputs, it
// performs a constraint solving analysis to develop a complete Solution, or
// else fail with an informative error.
//
// If a Solution is found, an implementing tool may persist it - typically into
// what a "lock file" - and/or use it to write out a directory tree of
// dependencies, suitable to be a vendor directory, via CreateVendorTree.
type Solver interface {
	// HashInputs produces a hash digest representing the unique inputs to this
	// solver. It is guaranteed that, if the hash digest is equal to the digest
	// from a previous Solution.InputHash(), that that Solution is valid for
	// this Solver's inputs.
	//
	// In such a case, it may not be necessary to run Solve() at all.
	HashInputs() ([]byte, error)
	Solve() (Solution, error)
}

// Prepare readies a Solver for use.
//
// This function reads and validates the provided SolveParameters. If a problem
// with the inputs is detected, an error is returned. Otherwise, a Solver is
// returned, ready to hash and check inputs or perform a solving run.
func Prepare(params SolveParameters, sm SourceManager) (Solver, error) {
	if sm == nil {
		return nil, badOptsFailure("must provide non-nil SourceManager")
	}
	if params.RootDir == "" {
		return nil, badOptsFailure("params must specify a non-empty root directory")
	}
	if params.ImportRoot == "" {
		return nil, badOptsFailure("params must include a non-empty import root")
	}
	if params.Trace && params.TraceLogger == nil {
		return nil, badOptsFailure("trace requested, but no logger provided")
	}

	if params.Manifest == nil {
		params.Manifest = simpleRootManifest{}
	}

	s := &solver{
		params: params,
		ig:     params.Manifest.IgnorePackages(),
		ovr:    params.Manifest.Overrides(),
		tl:     params.TraceLogger,
	}

	// Ensure the ignore and overrides maps are at least initialized
	if s.ig == nil {
		s.ig = make(map[string]bool)
	}
	if s.ovr == nil {
		s.ovr = make(ProjectConstraints)
	}

	// Validate no empties in the overrides map
	var eovr []string
	for pr, pp := range s.ovr {
		if pp.Constraint == nil && pp.NetworkName == "" {
			eovr = append(eovr, string(pr))
		}
	}

	if eovr != nil {
		// Maybe it's a little nitpicky to do this (we COULD proceed; empty
		// overrides have no effect), but this errs on the side of letting the
		// tool/user know there's bad input. Purely as a principle, that seems
		// preferable to silently allowing progress with icky input.
		if len(eovr) > 1 {
			return nil, badOptsFailure(fmt.Sprintf("Overrides lacked any non-zero properties for multiple project roots: %s", strings.Join(eovr, " ")))
		}
		return nil, badOptsFailure(fmt.Sprintf("An override was declared for %s, but without any non-zero properties", eovr[0]))
	}

	// Set up the bridge and ensure the root dir is in good, working order
	// before doing anything else. (This call is stubbed out in tests, via
	// overriding mkBridge(), so we can run with virtual RootDir.)
	s.b = mkBridge(s, sm)
	err := s.b.verifyRootDir(s.params.RootDir)
	if err != nil {
		return nil, err
	}

	// Initialize maps
	s.chng = make(map[ProjectRoot]struct{})
	s.rlm = make(map[ProjectIdentifier]LockedProject)
	s.names = make(map[ProjectRoot]string)

	for _, v := range s.params.ToChange {
		s.chng[v] = struct{}{}
	}

	// Initialize stacks and queues
	s.sel = &selection{
		deps: make(map[ProjectIdentifier][]dependency),
		sm:   s.b,
	}
	s.unsel = &unselected{
		sl:  make([]bimodalIdentifier, 0),
		cmp: s.unselectedComparator,
	}

	// Prep safe, normalized versions of root manifest and lock data
	s.rm = prepManifest(s.params.Manifest)
	if s.params.Lock != nil {
		for _, lp := range s.params.Lock.Projects() {
			s.rlm[lp.Ident().normalize()] = lp
		}

		// Also keep a prepped one, mostly for the bridge. This is probably
		// wasteful, but only minimally so, and yay symmetry
		s.rl = prepLock(s.params.Lock)
	}

	return s, nil
}

// Solve attempts to find a dependency solution for the given project, as
// represented by the SolveParameters with which this Solver was created.
//
// This is the entry point to the main gps workhorse.
func (s *solver) Solve() (Solution, error) {
	// Prime the queues with the root project
	err := s.selectRoot()
	if err != nil {
		return nil, err
	}

	all, err := s.solve()

	var soln solution
	if err == nil {
		soln = solution{
			att: s.attempts,
		}

		// An err here is impossible; it could only be caused by a parsing error
		// of the root tree, but that necessarily succeeded back up
		// selectRoot(), so we can ignore this err
		soln.hd, _ = s.HashInputs()

		// Convert ProjectAtoms into LockedProjects
		soln.p = make([]LockedProject, len(all))
		k := 0
		for pa, pl := range all {
			soln.p[k] = pa2lp(pa, pl)
			k++
		}
	}

	s.traceFinish(soln, err)
	return soln, err
}

// solve is the top-level loop for the SAT solving process.
func (s *solver) solve() (map[atom]map[string]struct{}, error) {
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
			queue, err := s.createVersionQueue(bmi)
			if err != nil {
				// Err means a failure somewhere down the line; try backtracking.
				s.traceStartBacktrack(bmi, err, false)
				//s.traceBacktrack(bmi, false)
				if s.backtrack() {
					// backtracking succeeded, move to the next unselected id
					continue
				}
				return nil, err
			}

			if queue.current() == nil {
				panic("canary - queue is empty, but flow indicates success")
			}

			awp := atomWithPackages{
				a: atom{
					id: queue.id,
					v:  queue.current(),
				},
				pl: bmi.pl,
			}
			s.selectAtom(awp, false)
			s.vqs = append(s.vqs, queue)
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
				a: atom{
					id: bmi.id,
					v:  awp.a.v,
				},
				pl: bmi.pl,
			}

			s.traceCheckPkgs(bmi)
			err := s.check(nawp, true)
			if err != nil {
				// Err means a failure somewhere down the line; try backtracking.
				s.traceStartBacktrack(bmi, err, true)
				if s.backtrack() {
					// backtracking succeeded, move to the next unselected id
					continue
				}
				return nil, err
			}
			s.selectAtom(nawp, true)
			// We don't add anything to the stack of version queues because the
			// backtracker knows not to pop the vqstack if it backtracks
			// across a pure-package addition.
		}
	}

	// Getting this far means we successfully found a solution. Combine the
	// selected projects and packages.
	projs := make(map[atom]map[string]struct{})

	// Skip the first project. It's always the root, and that shouldn't be
	// included in results.
	for _, sel := range s.sel.projects[1:] {
		pm, exists := projs[sel.a.a]
		if !exists {
			pm = make(map[string]struct{})
			projs[sel.a.a] = pm
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
	pa := atom{
		id: ProjectIdentifier{
			ProjectRoot: s.params.ImportRoot,
		},
		// This is a hack so that the root project doesn't have a nil version.
		// It's sort of OK because the root never makes it out into the results.
		// We may need a more elegant solution if we discover other side
		// effects, though.
		v: rootRev,
	}

	ptree, err := s.b.ListPackages(pa.id, nil)
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
		a:  pa,
		pl: list,
	}

	// Push the root project onto the queue.
	// TODO(sdboyer) maybe it'd just be better to skip this?
	s.sel.pushSelection(a, true)

	// If we're looking for root's deps, get it from opts and local root
	// analysis, rather than having the sm do it
	c, tc := s.rm.DependencyConstraints(), s.rm.TestDependencyConstraints()
	mdeps := s.ovr.overrideAll(pcSliceToMap(c, tc).asSortedSlice())

	// Err is not possible at this point, as it could only come from
	// listPackages(), which if we're here already succeeded for root
	reach, _ := s.b.computeRootReach()

	deps, err := s.intersectConstraintsWithImports(mdeps, reach)
	if err != nil {
		// TODO(sdboyer) this could well happen; handle it with a more graceful error
		panic(fmt.Sprintf("shouldn't be possible %s", err))
	}

	for _, dep := range deps {
		s.sel.pushDep(dependency{depender: pa, dep: dep})
		// Add all to unselected queue
		s.names[dep.Ident.ProjectRoot] = dep.Ident.netName()
		heap.Push(s.unsel, bimodalIdentifier{id: dep.Ident, pl: dep.pl, fromRoot: true})
	}

	s.traceSelectRoot(ptree, deps)
	return nil
}

func (s *solver) getImportsAndConstraintsOf(a atomWithPackages) ([]completeDep, error) {
	var err error

	if s.params.ImportRoot == a.a.id.ProjectRoot {
		panic("Should never need to recheck imports/constraints from root during solve")
	}

	// Work through the source manager to get project info and static analysis
	// information.
	m, _, err := s.b.GetManifestAndLock(a.a.id, a.a.v)
	if err != nil {
		return nil, err
	}

	ptree, err := s.b.ListPackages(a.a.id, a.a.v)
	if err != nil {
		return nil, err
	}

	allex := ptree.ExternalReach(false, false, s.ig)
	// Use a map to dedupe the unique external packages
	exmap := make(map[string]struct{})
	// Add the packages reached by the packages explicitly listed in the atom to
	// the list
	for _, pkg := range a.pl {
		expkgs, exists := allex[pkg]
		if !exists {
			// missing package here *should* only happen if the target pkg was
			// poisoned somehow - check the original ptree.
			if perr, exists := ptree.Packages[pkg]; exists {
				if perr.Err != nil {
					return nil, fmt.Errorf("package %s has errors: %s", pkg, perr.Err)
				}
				return nil, fmt.Errorf("package %s depends on some other package within %s with errors", pkg, a.a.id.errString())
			}
			// Nope, it's actually not there. This shouldn't happen.
			return nil, fmt.Errorf("package %s does not exist within project %s", pkg, a.a.id.errString())
		}

		for _, ex := range expkgs {
			exmap[ex] = struct{}{}
		}
	}

	reach := make([]string, len(exmap))
	k := 0
	for pkg := range exmap {
		reach[k] = pkg
		k++
	}

	deps := s.ovr.overrideAll(m.DependencyConstraints())

	return s.intersectConstraintsWithImports(deps, reach)
}

// intersectConstraintsWithImports takes a list of constraints and a list of
// externally reached packages, and creates a []completeDep that is guaranteed
// to include all packages named by import reach, using constraints where they
// are available, or Any() where they are not.
func (s *solver) intersectConstraintsWithImports(deps []workingConstraint, reach []string) ([]completeDep, error) {
	// Create a radix tree with all the projects we know from the manifest
	// TODO(sdboyer) make this smarter once we allow non-root inputs as 'projects'
	xt := radix.New()
	for _, dep := range deps {
		xt.Insert(string(dep.Ident.ProjectRoot), dep)
	}

	// Step through the reached packages; if they have prefix matches in
	// the trie, assume (mostly) it's a correct correspondence.
	dmap := make(map[ProjectRoot]completeDep)
	for _, rp := range reach {
		// If it's a stdlib package, skip it.
		// TODO(sdboyer) this just hardcodes us to the packages in tip - should we
		// have go version magic here, too?
		if stdlib[rp] {
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
			// The latter would incorrectly be conflated with the former. So, as
			// we know we're operating on strings that describe paths, guard
			// against this case by verifying that either the input is the same
			// length as the match (in which case we know they're equal), or
			// that the next character is the is the PathSeparator.
			if len(k) == len(rp) || strings.IndexRune(rp[:len(k)], os.PathSeparator) == 0 {
				// Match is valid; put it in the dmap, either creating a new
				// completeDep or appending it to the existing one for this base
				// project/prefix.
				dep := idep.(workingConstraint)
				if cdep, exists := dmap[dep.Ident.ProjectRoot]; exists {
					cdep.pl = append(cdep.pl, rp)
					dmap[dep.Ident.ProjectRoot] = cdep
				} else {
					dmap[dep.Ident.ProjectRoot] = completeDep{
						workingConstraint: dep,
						pl:                []string{rp},
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

		// Make a new completeDep with an open constraint, respecting overrides
		pd := s.ovr.override(ProjectConstraint{
			Ident: ProjectIdentifier{
				ProjectRoot: ProjectRoot(root.Base),
				NetworkName: root.Base,
			},
			Constraint: Any(),
		})

		// Insert the pd into the trie so that further deps from this
		// project get caught by the prefix search
		xt.Insert(root.Base, pd)
		// And also put the complete dep into the dmap
		dmap[ProjectRoot(root.Base)] = completeDep{
			workingConstraint: pd,
			pl:                []string{rp},
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
	if s.params.ImportRoot == id.ProjectRoot {
		return newVersionQueue(id, nil, nil, s.b)
	}

	exists, err := s.b.RepoExists(id)
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
			// TODO(sdboyer) mark this for special handling, somehow?
		} else {
			return nil, fmt.Errorf("Project '%s' could not be located.", id)
		}
	}

	var lockv Version
	if len(s.rlm) > 0 {
		lockv, err = s.getLockVersionIfValid(id)
		if err != nil {
			// Can only get an error here if an upgrade was expressly requested on
			// code that exists only in vendor
			return nil, err
		}
	}

	var prefv Version
	if bmi.fromRoot {
		// If this bmi came from the root, then we want to search through things
		// with a dependency on it in order to see if any have a lock that might
		// express a prefv
		//
		// TODO(sdboyer) nested loop; prime candidate for a cache somewhere
		for _, dep := range s.sel.getDependenciesOn(bmi.id) {
			// Skip the root, of course
			if s.params.ImportRoot == dep.depender.id.ProjectRoot {
				continue
			}

			_, l, err := s.b.GetManifestAndLock(dep.depender.id, dep.depender.v)
			if err != nil || l == nil {
				// err being non-nil really shouldn't be possible, but the lock
				// being nil is quite likely
				continue
			}

			for _, lp := range l.Projects() {
				if lp.Ident().eq(bmi.id) {
					prefv = lp.Version()
				}
			}
		}

		// OTHER APPROACH - WRONG, BUT MAYBE USEFUL FOR REFERENCE?
		// If this bmi came from the root, then we want to search the unselected
		// queue to see if anything *else* wants this ident, in which case we
		// pick up that prefv
		//for _, bmi2 := range s.unsel.sl {
		//// Take the first thing from the queue that's for the same ident,
		//// and has a non-nil prefv
		//if bmi.id.eq(bmi2.id) {
		//if bmi2.prefv != nil {
		//prefv = bmi2.prefv
		//}
		//}
		//}

	} else {
		// Otherwise, just use the preferred version expressed in the bmi
		prefv = bmi.prefv
	}

	q, err := newVersionQueue(id, lockv, prefv, s.b)
	if err != nil {
		// TODO(sdboyer) this particular err case needs to be improved to be ONLY for cases
		// where there's absolutely nothing findable about a given project name
		return nil, err
	}

	// Hack in support for revisions.
	//
	// By design, revs aren't returned from ListVersion(). Thus, if the dep in
	// the bmi was has a rev constraint, it is (almost) guaranteed to fail, even
	// if that rev does exist in the repo. So, detect a rev and push it into the
	// vq here, instead.
	//
	// Happily, the solver maintains the invariant that constraints on a given
	// ident cannot be incompatible, so we know that if we find one rev, then
	// any other deps will have to also be on that rev (or Any).
	//
	// TODO(sdboyer) while this does work, it bypasses the interface-implied guarantees
	// of the version queue, and is therefore not a great strategy for API
	// coherency. Folding this in to a formal interface would be better.
	switch tc := s.sel.getConstraint(bmi.id).(type) {
	case Revision:
		// We know this is the only thing that could possibly match, so put it
		// in at the front - if it isn't there already.
		if q.pi[0] != tc {
			// Existence of the revision is guaranteed by checkRevisionExists().
			q.pi = append([]Version{tc}, q.pi...)
		}
	}

	// Having assembled the queue, search it for a valid version.
	s.traceCheckQueue(q, bmi, false, 1)
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
		// this case should not be reachable, but reflects improper solver state
		// if it is, so panic immediately
		panic("version queue is empty, should not happen")
	}

	faillen := len(q.fails)

	for {
		cur := q.current()
		s.traceInfo("try %s@%s", q.id.errString(), cur)
		err := s.check(atomWithPackages{
			a: atom{
				id: q.id,
				v:  cur,
			},
			pl: pl,
		}, false)
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

	s.fail(s.sel.getDependenciesOn(q.id)[0].depender.id)

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
func (s *solver) getLockVersionIfValid(id ProjectIdentifier) (Version, error) {
	// If the project is specifically marked for changes, then don't look for a
	// locked version.
	if _, explicit := s.chng[id.ProjectRoot]; explicit || s.params.ChangeAll {
		// For projects with an upstream or cache repository, it's safe to
		// ignore what's in the lock, because there's presumably more versions
		// to be found and attempted in the repository. If it's only in vendor,
		// though, then we have to try to use what's in the lock, because that's
		// the only version we'll be able to get.
		if exist, _ := s.b.RepoExists(id); exist {
			return nil, nil
		}

		// However, if a change was *expressly* requested for something that
		// exists only in vendor, then that guarantees we don't have enough
		// information to complete a solution. In that case, error out.
		if explicit {
			return nil, &missingSourceFailure{
				goal: id,
				prob: "Cannot upgrade %s, as no source repository could be found.",
			}
		}
	}

	lp, exists := s.rlm[id]
	if !exists {
		return nil, nil
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
			return nil, nil
		}
	}

	return v, nil
}

// backtrack works backwards from the current failed solution to find the next
// solution to try.
func (s *solver) backtrack() bool {
	if len(s.vqs) == 0 {
		// nothing to backtrack to
		return false
	}

	for {
		for {
			if len(s.vqs) == 0 {
				// no more versions, nowhere further to backtrack
				return false
			}
			if s.vqs[len(s.vqs)-1].failed {
				break
			}

			s.vqs, s.vqs[len(s.vqs)-1] = s.vqs[:len(s.vqs)-1], nil

			// Pop selections off until we get to a project.
			var proj bool
			var awp atomWithPackages
			for !proj {
				awp, proj = s.unselectLast()
				s.traceBacktrack(awp.bmi(), !proj)
			}
		}

		// Grab the last versionQueue off the list of queues
		q := s.vqs[len(s.vqs)-1]

		// Walk back to the next project
		awp, proj := s.unselectLast()
		if !proj {
			panic("canary - *should* be impossible to have a pkg-only selection here")
		}

		if !q.id.eq(awp.a.id) {
			panic("canary - version queue stack and selected project stack are misaligned")
		}

		// Advance the queue past the current version, which we know is bad
		// TODO(sdboyer) is it feasible to make available the failure reason here?
		if q.advance(nil) == nil && !q.isExhausted() {
			// Search for another acceptable version of this failed dep in its queue
			s.traceCheckQueue(q, awp.bmi(), true, 0)
			if s.findValidVersion(q, awp.pl) == nil {
				// Found one! Put it back on the selected queue and stop
				// backtracking

				// reusing the old awp is fine
				awp.a.v = q.current()
				s.selectAtom(awp, false)
				break
			}
		}

		s.traceBacktrack(awp.bmi(), false)
		//s.traceInfo("no more versions of %s, backtracking", q.id.errString())

		// No solution found; continue backtracking after popping the queue
		// we just inspected off the list
		// GC-friendly pop pointer elem in slice
		s.vqs, s.vqs[len(s.vqs)-1] = s.vqs[:len(s.vqs)-1], nil
	}

	// Backtracking was successful if loop ended before running out of versions
	if len(s.vqs) == 0 {
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

	// We can safely ignore an err from ListVersions here because, if there is
	// an actual problem, it'll be noted and handled somewhere else saner in the
	// solving algorithm.
	ivl, _ := s.b.ListVersions(iname)
	jvl, _ := s.b.ListVersions(jname)
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
	// TODO(sdboyer) does this need updating, now that we have non-project package
	// selection?

	// skip if the root project
	if s.params.ImportRoot != id.ProjectRoot {
		// just look for the first (oldest) one; the backtracker will necessarily
		// traverse through and pop off any earlier ones
		for _, vq := range s.vqs {
			if vq.id.eq(id) {
				vq.failed = true
				return
			}
		}
	}
}

// selectAtom pulls an atom into the selection stack, alongside some of
// its contained packages. New resultant dependency requirements are added to
// the unselected priority queue.
//
// Behavior is slightly diffferent if pkgonly is true.
func (s *solver) selectAtom(a atomWithPackages, pkgonly bool) {
	s.unsel.remove(bimodalIdentifier{
		id: a.a.id,
		pl: a.pl,
	})

	s.sel.pushSelection(a, pkgonly)

	deps, err := s.getImportsAndConstraintsOf(a)
	if err != nil {
		// This shouldn't be possible; other checks should have ensured all
		// packages and deps are present for any argument passed to this method.
		panic(fmt.Sprintf("canary - shouldn't be possible %s", err))
	}

	// If this atom has a lock, pull it out so that we can potentially inject
	// preferred versions into any bmis we enqueue
	_, l, _ := s.b.GetManifestAndLock(a.a.id, a.a.v)
	var lmap map[ProjectIdentifier]Version
	if l != nil {
		lmap = make(map[ProjectIdentifier]Version)
		for _, lp := range l.Projects() {
			lmap[lp.Ident()] = lp.Version()
		}
	}

	for _, dep := range deps {
		s.sel.pushDep(dependency{depender: a.a, dep: dep})
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
			bmi := bimodalIdentifier{
				id: dep.Ident,
				pl: newp,
				// This puts in a preferred version if one's in the map, else
				// drops in the zero value (nil)
				prefv: lmap[dep.Ident],
			}
			heap.Push(s.unsel, bmi)
		}

		if s.sel.depperCount(dep.Ident) == 1 {
			s.names[dep.Ident.ProjectRoot] = dep.Ident.netName()
		}
	}

	s.traceSelect(a, pkgonly)
}

func (s *solver) unselectLast() (atomWithPackages, bool) {
	awp, first := s.sel.popSelection()
	heap.Push(s.unsel, bimodalIdentifier{id: awp.a.id, pl: awp.pl})

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
			delete(s.names, dep.Ident.ProjectRoot)
			s.unsel.remove(bimodalIdentifier{id: dep.Ident, pl: dep.pl})
		}
	}

	return awp, first
}

// simple (temporary?) helper just to convert atoms into locked projects
func pa2lp(pa atom, pkgs map[string]struct{}) LockedProject {
	lp := LockedProject{
		pi: pa.id.normalize(), // shouldn't be necessary, but normalize just in case
	}

	switch v := pa.v.(type) {
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
		lp.pkgs = append(lp.pkgs, strings.TrimPrefix(pkg, string(pa.id.ProjectRoot)+string(os.PathSeparator)))
	}
	sort.Strings(lp.pkgs)

	return lp
}
