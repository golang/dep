package vsolver

import (
	"container/heap"
	"fmt"
)

type SolveFailure uint

const (
	// Indicates that no version solution could be found
	NoVersionSolution SolveFailure = 1 << iota
	IncompatibleVersionType
)

func NewSolver(pf PackageFetcher) Solver {
	return &solver{
		pf:  pf,
		sel: &selection{},
	}
}

type solver struct {
	pf       PackageFetcher
	latest   map[ProjectIdentifier]struct{}
	sel      *selection
	unsel    *unselected
	versions []*VersionQueue
	rs       Spec
	rl       Lock
}

func (s *solver) Solve(rootSpec Spec, rootLock Lock, toUpgrade []ProjectIdentifier) Result {
	// local overrides would need to be handled first. ofc, these don't exist yet

	for _, v := range toUpgrade {
		s.latest[v] = struct{}{}
	}

	s.unsel = &unselected{
		sl:  make([]ProjectIdentifier, 0),
		cmp: s.unselectedComparator,
	}
	heap.Init(s.unsel)

	s.rs = rootSpec
	s.rl = rootLock

	_, err := s.doSolve()
}

func (s *solver) doSolve() ([]ProjectID, error) {
	for {
		ref := s.sel.nextUnselected()
		if ref == "" {
			// no more packages to select - we're done. bail out
			// TODO compile things in s.sel into a list of ProjectIDs, and return
			break
		}

		queue, err := s.createVersionQueue(ref)

		if err != nil {
			// Err means a failure somewhere down the line; try backtracking.
			if s.backtrack() {
				// backtracking succeeded, move to the next unselected ref
				continue
			}
			// TODO handle failures, lolzies
		}
	}
}

func (s *solver) createVersionQueue(ref ProjectIdentifier) (*VersionQueue, error) {
	// If on the root package, there's no queue to make
	if ref == s.rs.ID {
		return NewVersionQueue(ref, nil, s.pf)
	}

	if !s.pf.ProjectExists(ref) {
		// TODO this check needs to incorporate/admit the possibility that the
		// upstream no longer exists, but there's something valid in vendor/
		return nil, newSolveError(fmt.Sprintf("Project '%s' could not be located.", ref), cannotResolve)
	}
	lockv := s.getLockVersionIfValid(ref)

	versions, err := s.pf.ListVersions(ref)
	if err != nil {
		// TODO can there actually be an err here? probably just e.g. an
		// fs-level err
		return nil, err // pass it straight back up
	}

	//var list []*ProjectID
	//for _, pi := range versions {
	//_, err := semver.NewVersion(pi.Version)
	//if err != nil {
	//// couldn't parse version; moving on
	//// TODO log this at all? would be info/debug-type, at best
	//continue
	//}
	//// this is the lockv, push it to the front
	//if lockv.Version == pi.Version {
	//list = append([]*ProjectID{&pi}, list...)
	//} else {
	//list = append(list, &pi)
	//}
	//}

	q, err := NewVersionQueue(ref, lockv, s.pf)
	if err != nil {
		// TODO this particular err case needs to be improved to be ONLY for cases
		// where there's absolutely nothing findable about a given project name
		return nil, err
	}

	return q, s.findValidVersion(q)
}

// findValidVersion walks through a VersionQueue until it finds a version that's
// valid, as adjudged by the current constraints.
func (s *solver) findValidVersion(q *VersionQueue) error {
	var err error
	if q.current() == nil {
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
			// Error on advance; have to bail out
			break
		}
	}

	s.fail(s.sel.getDependenciesOn(q.current().ID)[0].Depender.ID)
	return err
}

func (s *solver) getLockVersionIfValid(ref ProjectIdentifier) *ProjectID {
	lockver := s.rl.GetProject(ref)
	if lockver == nil {
		// Nothing in the lock about this version, so nothing to validate
		return nil
	}

	constraint := s.sel.getConstraint(ref)
	if !constraint.Allows(lockver.Version) {
		// TODO msg?
		return nil
		//} else {
		// TODO msg?
	}

	return nil
}

// getAllowedVersions retrieves an ordered list of versions from the source manager for
// the given identifier. It returns an error if the named project does not exist.
//
// ...REALLY NOT NECESSARY, VERSIONQUEUE CAN JUST DO IT DIRECTLY?
//
//func (s *solver) getAllowedVersions(ref ProjectIdentifier) (ids []*ProjectID, err error) {
//ids, err = s.pf.ListVersions(ref)
//if err != nil {
//// TODO ...more err handling here?
//return nil, err
//}
//}

func (s *solver) checkVersion(pi *ProjectID) error {
	if pi == nil {
		// TODO we should protect against this case elsewhere, but for now panic
		// to canary when it's a problem
		panic("checking version of nil ProjectID pointer")
	}

	constraint := s.sel.getConstraint(pi.ID)
	if !constraint.Allows(pi.Version) {
		deps := s.sel.getDependenciesOn(pi.ID)
		for _, dep := range deps {
			// TODO grok why this check is needed
			if !dep.Dep.Constraint.Allows(pi.Version) {
				s.fail(dep.Depender.ID)
			}
		}

		// TODO msg
		return &noVersionError{
			pi:   *pi,
			c:    constraint,
			deps: deps,
		}
	}

	if !s.pf.ProjectExists(pi.ID) {
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
		selfAndSiblings := append(s.sel.getDependenciesOn(dep.ID), Dependency{Depender: *pi, Dep: dep})

		constraint = s.sel.getConstraint(dep.ID)
		// Ensure the constraint expressed by the dep has at least some possible
		// overlap with existing constraints.
		if !constraint.Intersects(dep.Constraint) {
			// No match - visit all siblings and identify the disagreement(s)
			for _, sibling := range selfAndSiblings[:len(selfAndSiblings)-1] {
				if !sibling.Dep.Constraint.Intersects(dep.Constraint) {
					s.fail(sibling.Depender.ID)
				}
			}

			// TODO msg
			return &disjointConstraintFailure{
				id:   dep.ID,
				deps: selfAndSiblings,
			}
		}

		selected := s.sel.selected(dep.ID)
		if selected != nil && !dep.Constraint.Allows(selected.Version) {
			s.fail(dep.ID)

			// TODO msg
			return &noVersionError{
				pi:   dep.ProjectID,
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
func (s *solver) getDependenciesOf(pi *ProjectID) ([]ProjectDep, error) {
	info, err := s.pf.GetProjectInfo(pi.ID)
	if err != nil {
		// TODO revisit this once a decision is made about better-formed errors;
		// question is, do we expect the fetcher to pass back simple errors, or
		// well-typed solver errors?
		return nil, err
	}

	deps := info.GetDependencies()
	if s.rs.ID == pi.ID {
		// Root package has more things to pull in
		deps = append(deps, info.GetDevDependencies())

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
			s.sel.unselectLast()
		}

		var pi *ProjectID
		var q *VersionQueue

		q := s.versions[len(s.versions)-1]
		id := q.current().ID
		// another assert that the last in s.sel's ids is == q.current
		s.sel.unselectLast()
	}
}

func (s *solver) unselectedComparator(i, j int) bool {
	iname, jname := s.unsel.sl[i], s.unsel.sl[j]

	if iname == jname {
		return false
	}

	// *always* put root project first
	if iname == s.rs.ID {
		return true
	}
	if jname == s.rs.ID {
		return false
	}

	ilock, jlock := s.rl.GetProject(iname) == nil, s.rl.GetProject(jname) == nil

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

}

func (s *solver) choose(id ProjectID) {

}
