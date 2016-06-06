package vsolver

import (
	"container/heap"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
)

var (
	// With a random revision and no name, collisions are unlikely
	nilpa = ProjectAtom{
		Version: Revision(strconv.FormatInt(rand.Int63(), 36)),
	}
)

type Solver interface {
	Solve(opts SolveOpts) (Result, error)
}

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
}

func NewSolver(sm SourceManager, l *log.Logger) Solver {
	return &solver{
		b:  &bridge{sm: sm},
		tl: l,
	}
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
	b *bridge

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
	versions []*versionQueue

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
// This is the entry point to vsolver's main workhorse.
func (s *solver) Solve(opts SolveOpts) (Result, error) {
	// local overrides would need to be handled first.
	// TODO local overrides! heh

	if opts.M == nil {
		return result{}, BadOptsFailure("Opts must include a manifest.")
	}
	if opts.Root == "" {
		return result{}, BadOptsFailure("Opts must specify a non-empty string for the project root directory.")
	}
	if opts.N == "" {
		return result{}, BadOptsFailure("Opts must include a project name.")
	}

	// TODO this check needs to go somewhere, but having the solver interact
	// directly with the filesystem is icky
	//if fi, err := os.Stat(opts.Root); err != nil {
	//return Result{}, fmt.Errorf("Project root must exist.")
	//} else if !fi.IsDir() {
	//return Result{}, fmt.Errorf("Project root must be a directory.")
	//}

	// Init/reset the smAdapter
	s.b.sortdown = opts.Downgrade
	s.b.vlists = make(map[ProjectName][]Version)

	s.o = opts

	// Force trace to false if no real logger was provided.
	if s.tl == nil {
		s.o.Trace = false
	}

	// Initialize maps
	s.chng = make(map[ProjectName]struct{})
	s.rlm = make(map[ProjectIdentifier]LockedProject)
	s.names = make(map[ProjectName]string)

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

	// Initialize queues
	s.sel = &selection{
		deps: make(map[ProjectIdentifier][]Dependency),
		sm:   s.b,
	}
	s.unsel = &unselected{
		sl:  make([]ProjectIdentifier, 0),
		cmp: s.unselectedComparator,
	}

	// Prime the queues with the root project
	s.selectVersion(ProjectAtom{
		Ident: ProjectIdentifier{
			LocalName: s.o.N,
		},
		// This is a hack so that the root project doesn't have a nil version.
		// It's sort of OK because the root never makes it out into the results.
		// We may need a more elegant solution if we discover other side
		// effects, though.
		Version: Revision(""),
	})

	// Prep is done; actually run the solver
	s.logSolve()
	pa, err := s.solve()

	// Solver finished with an err; return that and we're done
	if err != nil {
		return nil, err
	}

	// Solved successfully, create and return a result
	r := result{
		att: s.attempts,
		hd:  opts.HashInputs(),
	}

	// Convert ProjectAtoms into LockedProjects
	r.p = make([]LockedProject, len(pa))
	for k, p := range pa {
		r.p[k] = pa2lp(p)
	}

	return r, nil
}

func (s *solver) solve() ([]ProjectAtom, error) {
	for {
		id, has := s.nextUnselected()

		if !has {
			// no more packages to select - we're done. bail out
			break
		}

		s.logStart(id)
		queue, err := s.createVersionQueue(id)

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

		s.selectVersion(ProjectAtom{
			Ident:   queue.id,
			Version: queue.current(),
		})
		s.versions = append(s.versions, queue)
		s.logSolve()
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

func (s *solver) createVersionQueue(id ProjectIdentifier) (*versionQueue, error) {
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

	return q, s.findValidVersion(q)
}

// findValidVersion walks through a versionQueue until it finds a version that
// satisfies the constraints held in the current state of the solver.
func (s *solver) findValidVersion(q *versionQueue) error {
	if nil == q.current() {
		// TODO this case shouldn't be reachable, but panic here as a canary
		panic("version queue is empty, should not happen")
	}

	faillen := len(q.fails)

	for {
		cur := q.current()
		err := s.satisfiable(ProjectAtom{
			Ident:   q.id,
			Version: cur,
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
		deps = append(s.rm.GetDependencies(), s.rm.GetDevDependencies()...)
	} else {
		info, err := s.b.getProjectInfo(pa)
		if err != nil {
			// TODO revisit this once a decision is made about better-formed errors;
			// question is, do we expect the fetcher to pass back simple errors, or
			// well-typed solver errors?
			return nil, err
		}

		deps = info.GetDependencies()
		// TODO add overrides here...if we impl the concept (which we should)
	}

	// TODO we have to validate well-formedness of a project's manifest
	// somewhere. this may be a good spot. alternatively, the fetcher may
	// validate well-formedness, whereas here we validate availability of the
	// named deps here. (the latter is sorta what pub does here)

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

			// pub asserts here that the last in s.sel's ids is == q.current
			s.versions, s.versions[len(s.versions)-1] = s.versions[:len(s.versions)-1], nil
			s.unselectLast()
		}

		// Grab the last versionQueue off the list of queues
		q := s.versions[len(s.versions)-1]

		// another assert that the last in s.sel's ids is == q.current
		s.unselectLast()

		// Advance the queue past the current version, which we know is bad
		// TODO is it feasible to make available the failure reason here?
		if q.advance(nil) == nil && !q.isExhausted() {
			// Search for another acceptable version of this failed dep in its queue
			if s.findValidVersion(q) == nil {
				s.logSolve()

				// Found one! Put it back on the selected queue and stop
				// backtracking
				s.selectVersion(ProjectAtom{
					Ident:   q.id,
					Version: q.current(),
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

func (s *solver) nextUnselected() (ProjectIdentifier, bool) {
	if len(s.unsel.sl) > 0 {
		return s.unsel.sl[0], true
	}

	return ProjectIdentifier{}, false
}

func (s *solver) unselectedComparator(i, j int) bool {
	iname, jname := s.unsel.sl[i], s.unsel.sl[j]

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

func (s *solver) selectVersion(pa ProjectAtom) {
	s.unsel.remove(pa.Ident)
	s.sel.projects = append(s.sel.projects, pa)

	deps, err := s.getDependenciesOf(pa)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic(fmt.Sprintf("shouldn't be possible %s", err))
	}

	for _, dep := range deps {
		siblingsAndSelf := append(s.sel.getDependenciesOn(dep.Ident), Dependency{Depender: pa, Dep: dep})
		s.sel.setDependenciesOn(dep.Ident, siblingsAndSelf)

		// add project to unselected queue if this is the first dep on it -
		// otherwise it's already in there, or been selected
		if len(siblingsAndSelf) == 1 {
			s.names[dep.Ident.LocalName] = dep.Ident.netName()
			heap.Push(s.unsel, dep.Ident)
		}
	}
}

func (s *solver) unselectLast() {
	var pa ProjectAtom
	pa, s.sel.projects = s.sel.projects[len(s.sel.projects)-1], s.sel.projects[:len(s.sel.projects)-1]
	heap.Push(s.unsel, pa.Ident)

	deps, err := s.getDependenciesOf(pa)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic("shouldn't be possible")
	}

	for _, dep := range deps {
		siblings := s.sel.getDependenciesOn(dep.Ident)
		siblings = siblings[:len(siblings)-1]
		s.sel.deps[dep.Ident] = siblings

		// if no siblings, remove from unselected queue
		if len(siblings) == 0 {
			delete(s.names, dep.Ident.LocalName)
			s.unsel.remove(dep.Ident)
		}
	}
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
