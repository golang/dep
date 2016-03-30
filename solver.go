package vsolver

import (
	"container/heap"
	"fmt"
)

//type SolveFailure uint

//const (
// Indicates that no version solution could be found
//NoVersionSolution SolveFailure = 1 << iota
//IncompatibleVersionType
//)

func NewSolver(sm SourceManager) Solver {
	return &solver{
		sm: sm,
	}
}

// solver is a backtracking-style SAT solver.
type solver struct {
	sm       SourceManager
	latest   map[ProjectIdentifier]struct{}
	sel      *selection
	unsel    *unselected
	versions []*versionQueue
	rp       ProjectInfo
	attempts int
}

func (s *solver) Solve(root ProjectInfo, toUpgrade []ProjectIdentifier) Result {
	// local overrides would need to be handled first.
	// TODO local overrides! heh
	s.rp = root

	for _, v := range toUpgrade {
		s.latest[v] = struct{}{}
	}

	// Initialize queues
	s.sel = &selection{
		deps: make(map[ProjectIdentifier][]Dependency),
	}
	s.unsel = &unselected{
		sl:  make([]ProjectIdentifier, 0),
		cmp: s.unselectedComparator,
	}
	heap.Init(s.unsel)

	// Prime the queues with the root project
	s.selectVersion(s.rp.pi)

	// Prep is done; actually run the solver
	var r Result
	r.Projects, r.SolveFailure = s.solve()
	return r
}

func (s *solver) solve() ([]ProjectID, error) {
	for {
		ref, has := s.nextUnselected()
		if !has {
			// no more packages to select - we're done. bail out
			break
		}

		queue, err := s.createVersionQueue(ref)

		if err != nil {
			// Err means a failure somewhere down the line; try backtracking.
			if s.backtrack() {
				// backtracking succeeded, move to the next unselected ref
				continue
			}
			// TODO handle different failure types appropriately, lolzies
			return nil, err
		}

		if queue.current() == emptyPID {
			panic("canary - queue is empty, but flow indicates success")
		}

		s.selectVersion(queue.current())
		s.versions = append(s.versions, queue)
	}

	// Getting this far means we successfully found a solution
	var projs []ProjectID
	for _, p := range s.sel.projects {
		projs = append(projs, p)
	}
	return projs, nil
}

func (s *solver) createVersionQueue(ref ProjectIdentifier) (*versionQueue, error) {
	// If on the root package, there's no queue to make
	if ref == s.rp.ID() {
		return newVersionQueue(ref, nil, s.sm)
	}

	if !s.sm.ProjectExists(ref) {
		// TODO this check needs to incorporate/admit the possibility that the
		// upstream no longer exists, but there's something valid in vendor/
		return nil, newSolveError(fmt.Sprintf("Project '%s' could not be located.", ref), cannotResolve)
	}
	lockv := s.getLockVersionIfValid(ref)

	q, err := newVersionQueue(ref, lockv, s.sm)
	if err != nil {
		// TODO this particular err case needs to be improved to be ONLY for cases
		// where there's absolutely nothing findable about a given project name
		return nil, err
	}

	return q, s.findValidVersion(q)
}

// findValidVersion walks through a VersionQueue until it finds a version that's
// valid, as adjudged by the current constraints.
func (s *solver) findValidVersion(q *versionQueue) error {
	var err error
	if emptyPID == q.current() {
		// TODO this case shouldn't be reachable, but panic here as a canary
		panic("version queue is empty, should not happen")
	}

	// TODO worth adding an isEmpty()-type method to VersionQueue?
	for {
		err = s.checkVersion(q.current())
		if err == nil {
			// we have a good version, can return safely
			return nil
		}

		err = q.advance()
		if err != nil {
			// Error on advance, have to bail out
			break
		}
		if q.isExhausted() {
			// Queue is empty, bail with error
			err = newSolveError(fmt.Sprintf("Exhausted queue for %q without finding a satisfactory version.", q.ref), mustResolve)
			break
		}
	}

	s.fail(s.sel.getDependenciesOn(q.ref)[0].Depender.ID)
	return err
}

func (s *solver) getLockVersionIfValid(ref ProjectIdentifier) *ProjectID {
	lockver := s.rp.GetProjectID(ref)
	if lockver == nil {
		// Nothing in the lock about this version, so nothing to validate
		return nil
	}

	constraint := s.sel.getConstraint(ref)
	if !constraint.Admits(lockver.Version) {
		// TODO msg?
		return nil
		//} else {
		// TODO msg?
	}

	return nil
}

func (s *solver) checkVersion(pi ProjectID) error {
	if emptyPID == pi {
		// TODO we should protect against this case elsewhere, but for now panic
		// to canary when it's a problem
		panic("checking version of empty ProjectID")
	}

	constraint := s.sel.getConstraint(pi.ID)
	if !constraint.Admits(pi.Version) {
		deps := s.sel.getDependenciesOn(pi.ID)
		for _, dep := range deps {
			// TODO grok why this check is needed
			if !dep.Dep.Constraint.Admits(pi.Version) {
				s.fail(dep.Depender.ID)
			}
		}

		// TODO msg
		return &noVersionError{
			pi:   pi.ID,
			c:    constraint,
			deps: deps,
		}
	}

	if !s.sm.ProjectExists(pi.ID) {
		// Can get here if the lock file specifies a now-nonexistent project
		// TODO this check needs to incorporate/accept the possibility that the
		// upstream no longer exists, but there's something valid in vendor/
		return newSolveError(fmt.Sprintf("Project '%s' could not be located.", pi.ID), cannotResolve)
	}

	deps, err := s.getDependenciesOf(pi)
	if err != nil {
		// An err here would be from the package fetcher; pass it straight back
		return err
	}

	for _, dep := range deps {
		// TODO dart skips "magic" deps here; do we need that?

		// TODO maybe differentiate between the confirmed items on the list, and
		// the one we're speculatively adding? or it may be fine b/c we know
		// it's the last one
		selfAndSiblings := append(s.sel.getDependenciesOn(dep.ID), Dependency{Depender: pi, Dep: dep})

		constraint = s.sel.getConstraint(dep.ID)
		// Ensure the constraint expressed by the dep has at least some possible
		// overlap with existing constraints.
		if !constraint.AdmitsAny(dep.Constraint) {
			// No match - visit all siblings and identify the disagreement(s)
			for _, sibling := range selfAndSiblings[:len(selfAndSiblings)-1] {
				if !sibling.Dep.Constraint.AdmitsAny(dep.Constraint) {
					s.fail(sibling.Depender.ID)
				}
			}

			// TODO msg
			return &disjointConstraintFailure{
				id:   dep.ID,
				deps: selfAndSiblings,
			}
		}

		selected, exists := s.sel.selected(dep.ID)
		if exists && !dep.Constraint.Admits(selected.Version) {
			s.fail(dep.ID)

			// TODO msg
			return &noVersionError{
				pi:   dep.ID,
				c:    dep.Constraint,
				deps: selfAndSiblings,
			}
		}

		// At this point, dart/pub do things related to 'required' dependencies,
		// which is about solving loops (i think) and so mostly not something we
		// have to care about.
	}

	return nil
}

// getDependenciesOf returns the dependencies of the given ProjectID, mediated
// through any overrides dictated by the root project.
//
// If it's the root project, also includes dev dependencies, etc.
func (s *solver) getDependenciesOf(pi ProjectID) ([]ProjectDep, error) {
	info, err := s.sm.GetProjectInfo(pi)
	if err != nil {
		// TODO revisit this once a decision is made about better-formed errors;
		// question is, do we expect the fetcher to pass back simple errors, or
		// well-typed solver errors?
		return nil, err
	}

	deps := info.GetDependencies()
	if s.rp.ID() == pi.ID {
		// Root package has more things to pull in
		deps = append(deps, info.GetDevDependencies()...)

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
			// pop last vqueue off of versions
			//q, s.versions := s.versions[len(s.versions)-1], s.versions[:len(s.versions)-1]
			// pub asserts here that the last in s.sel's ids is == q.current
			s.versions = s.versions[:len(s.versions)-1]
			s.unselectLast()
		}

		// Grab the last VersionQueue off the list of queues
		q := s.versions[len(s.versions)-1]
		// another assert that the last in s.sel's ids is == q.current
		s.unselectLast()

		// Search for another acceptable version of this failed dep in its queue
		if err := s.findValidVersion(q); err == nil {
			// Found one! Put it back on the selected queue and stop
			// backtracking
			s.selectVersion(q.current())
			break
		}

		// No solution found; continue backtracking after popping the last
		// version off the list
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

	return "", false
}

func (s *solver) unselectedComparator(i, j int) bool {
	iname, jname := s.unsel.sl[i], s.unsel.sl[j]

	if iname == jname {
		return false
	}

	rid := s.rp.ID()
	// *always* put root project first
	if iname == rid {
		return true
	}
	if jname == rid {
		return false
	}

	ilock, jlock := s.rp.GetProjectID(iname) == nil, s.rp.GetProjectID(jname) == nil

	if ilock && !jlock {
		return true
	}
	if !ilock && jlock {
		return false
	}
	//if ilock && jlock {
	//return iname < jname
	//}

	// TODO impl version-counting for next set of checks. but until then...
	return iname < jname
}

func (s *solver) fail(id ProjectIdentifier) {
	// skip if the root project
	if s.rp.ID() == id {
		return
	}

	for _, vq := range s.versions {
		if vq.ref == id {
			vq.failed = true
			// just look for the first (oldest) one; the backtracker will
			// necessarily traverse through and pop off any earlier ones
			// TODO ...right?
			return
		}
	}
}

func (s *solver) selectVersion(id ProjectID) {
	s.unsel.remove(id.ID)
	s.sel.projects = append(s.sel.projects, id)

	deps, err := s.getDependenciesOf(id)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic("shouldn't be possible")
	}

	for _, dep := range deps {
		siblingsAndSelf := append(s.sel.getDependenciesOn(dep.ID), Dependency{Depender: id, Dep: dep})
		s.sel.deps[dep.ID] = siblingsAndSelf

		// add project to unselected queue if this is the first dep on it -
		// otherwise it's already in there, or been selected
		if len(siblingsAndSelf) == 1 {
			heap.Push(s.unsel, dep.ID)
		}
	}
}

func (s *solver) unselectLast() {
	var id ProjectID
	id, s.sel.projects = s.sel.projects[len(s.sel.projects)-1], s.sel.projects[:len(s.sel.projects)-1]
	heap.Push(s.unsel, id.ID)

	deps, err := s.getDependenciesOf(id)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic("shouldn't be possible")
	}

	for _, dep := range deps {
		siblings := s.sel.getDependenciesOn(id.ID)
		s.sel.deps[id.ID] = siblings[:len(siblings)-1]

		// if no siblings, remove from unselected queue
		if len(siblings) == 0 {
			s.unsel.remove(dep.ID)
		}
	}
}
