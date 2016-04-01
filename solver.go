package vsolver

import (
	"container/heap"
	"fmt"

	"github.com/Sirupsen/logrus"
)

//type SolveFailure uint

//const (
// Indicates that no version solution could be found
//NoVersionSolution SolveFailure = 1 << iota
//IncompatibleVersionType
//)

func NewSolver(sm SourceManager, l *logrus.Logger) Solver {
	if l == nil {
		l = logrus.New()
	}

	return &solver{
		sm: sm,
		l:  l,
	}
}

// solver is a backtracking-style SAT solver.
type solver struct {
	l        *logrus.Logger
	sm       SourceManager
	latest   map[ProjectName]struct{}
	sel      *selection
	unsel    *unselected
	versions []*versionQueue
	rp       ProjectInfo
	attempts int
}

func (s *solver) Solve(root ProjectInfo, toUpgrade []ProjectName) Result {
	// local overrides would need to be handled first.
	// TODO local overrides! heh
	s.rp = root

	for _, v := range toUpgrade {
		s.latest[v] = struct{}{}
	}

	// Initialize queues
	s.sel = &selection{
		deps: make(map[ProjectName][]Dependency),
	}
	s.unsel = &unselected{
		sl:  make([]ProjectName, 0),
		cmp: s.unselectedComparator,
	}
	heap.Init(s.unsel)

	// Prime the queues with the root project
	s.selectVersion(s.rp.pa)

	// Prep is done; actually run the solver
	var r Result
	r.Projects, r.SolveFailure = s.solve()
	return r
}

func (s *solver) solve() ([]ProjectAtom, error) {
	for {
		ref, has := s.nextUnselected()

		if !has {
			// no more packages to select - we're done. bail out
			break
		}

		if s.l.Level >= logrus.DebugLevel {
			s.l.WithFields(logrus.Fields{
				"attempts": s.attempts,
				"name":     ref,
				"selcount": len(s.sel.projects),
			}).Debug("Beginning step in solve loop")
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

		if queue.current() == emptyVersion {
			panic("canary - queue is empty, but flow indicates success")
		}

		if s.l.Level >= logrus.InfoLevel {
			s.l.WithFields(logrus.Fields{
				"name":    queue.ref,
				"version": queue.current().Info,
			}).Info("Accepted project atom")
		}

		s.selectVersion(ProjectAtom{
			Name:    queue.ref,
			Version: queue.current(),
		})
		s.versions = append(s.versions, queue)
	}

	// Getting this far means we successfully found a solution
	var projs []ProjectAtom
	for _, p := range s.sel.projects {
		projs = append(projs, p)
	}
	return projs, nil
}

func (s *solver) createVersionQueue(ref ProjectName) (*versionQueue, error) {
	// If on the root package, there's no queue to make
	if ref == s.rp.Name() {
		return newVersionQueue(ref, nil, s.sm)
	}

	if !s.sm.ProjectExists(ref) {
		// TODO this check needs to incorporate/admit the possibility that the
		// upstream no longer exists, but there's something valid in vendor/
		if s.l.Level >= logrus.WarnLevel {
			s.l.WithFields(logrus.Fields{
				"name": ref,
			}).Warn("Upstream project does not exist")
		}
		return nil, newSolveError(fmt.Sprintf("Project '%s' could not be located.", ref), cannotResolve)
	}
	lockv := s.getLockVersionIfValid(ref)

	q, err := newVersionQueue(ref, lockv, s.sm)
	if err != nil {
		// TODO this particular err case needs to be improved to be ONLY for cases
		// where there's absolutely nothing findable about a given project name
		if s.l.Level >= logrus.WarnLevel {
			s.l.WithFields(logrus.Fields{
				"name": ref,
				"err":  err,
			}).Warn("Failed to create a version queue")
		}
		return nil, err
	}

	if s.l.Level >= logrus.DebugLevel {
		if lockv == nil {
			s.l.WithFields(logrus.Fields{
				"name": ref,
			}).Debug("Created VersionQueue, but no data in lock for project")
		} else {
			s.l.WithFields(logrus.Fields{
				"name":  ref,
				"lockv": lockv.Version.Info,
			}).Debug("Created VersionQueue using version found in lock")
		}
	}

	return q, s.findValidVersion(q)
}

// findValidVersion walks through a VersionQueue until it finds a version that's
// valid, as adjudged by the current constraints.
func (s *solver) findValidVersion(q *versionQueue) error {
	var err error
	if emptyVersion == q.current() {
		// TODO this case shouldn't be reachable, but panic here as a canary
		panic("version queue is empty, should not happen")
	}

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"name":      q.ref,
			"hasLock":   q.hasLock,
			"allLoaded": q.allLoaded,
		}).Debug("Beginning search through VersionQueue for a valid version")
	}

	for {
		err = s.checkVersion(ProjectAtom{
			Name:    q.ref,
			Version: q.current(),
		})
		if err == nil {
			// we have a good version, can return safely
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithFields(logrus.Fields{
					"name":    q.ref,
					"version": q.current().Info,
				}).Debug("Found acceptable version, returning out")
			}
			return nil
		}

		err = q.advance()
		if err != nil {
			// Error on advance, have to bail out
			if s.l.Level >= logrus.WarnLevel {
				s.l.WithFields(logrus.Fields{
					"name": q.ref,
					"err":  err,
				}).Warn("Advancing version queue returned unexpected error, marking project as failed")
			}
			break
		}
		if q.isExhausted() {
			// Queue is empty, bail with error
			err = newSolveError(fmt.Sprintf("Exhausted queue for %q without finding a satisfactory version.", q.ref), mustResolve)
			if s.l.Level >= logrus.InfoLevel {
				s.l.WithFields(logrus.Fields{
					"name": q.ref,
					"err":  err,
				}).Info("Version queue was completely exhausted, marking project as failed")
			}
			break
		}
	}

	s.fail(s.sel.getDependenciesOn(q.ref)[0].Depender.Name)
	return err
}

func (s *solver) getLockVersionIfValid(ref ProjectName) *ProjectAtom {
	lockver := s.rp.GetProjectAtom(ref)
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

func (s *solver) checkVersion(pi ProjectAtom) error {
	if emptyProjectAtom == pi {
		// TODO we should protect against this case elsewhere, but for now panic
		// to canary when it's a problem
		panic("checking version of empty ProjectAtom")
	}

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"name":    pi.Name,
			"version": pi.Version.Info,
		}).Debug("Checking acceptability of project atom against current constraints")
	}

	constraint := s.sel.getConstraint(pi.Name)
	if !constraint.Admits(pi.Version) {
		// TODO collect constraint failure reason

		if s.l.Level >= logrus.InfoLevel {
			s.l.WithFields(logrus.Fields{
				"name":          pi.Name,
				"version":       pi.Version.Info,
				"curconstraint": constraint.Body(),
			}).Info("Current constraints do not allow version")
		}

		deps := s.sel.getDependenciesOn(pi.Name)
		for _, dep := range deps {
			if !dep.Dep.Constraint.Admits(pi.Version) {
				if s.l.Level >= logrus.DebugLevel {
					s.l.WithFields(logrus.Fields{
						"name":       pi.Name,
						"othername":  dep.Depender.Name,
						"constraint": dep.Dep.Constraint.Body(),
					}).Debug("Marking other, selected project with conflicting constraint as failed")
				}
				s.fail(dep.Depender.Name)
			}
		}

		// TODO msg
		return &noVersionError{
			pn:   pi.Name,
			c:    constraint,
			deps: deps,
		}
	}

	if !s.sm.ProjectExists(pi.Name) {
		// Can get here if the lock file specifies a now-nonexistent project
		// TODO this check needs to incorporate/accept the possibility that the
		// upstream no longer exists, but there's something valid in vendor/
		return newSolveError(fmt.Sprintf("Project '%s' could not be located.", pi.Name), cannotResolve)
	}

	deps, err := s.getDependenciesOf(pi)
	if err != nil {
		// An err here would be from the package fetcher; pass it straight back
		return err
	}

	for _, dep := range deps {
		// TODO dart skips "magic" deps here; do we need that?

		siblings := s.sel.getDependenciesOn(dep.Name)

		constraint = s.sel.getConstraint(dep.Name)
		// Ensure the constraint expressed by the dep has at least some possible
		// intersection with the intersection of existing constraints.
		if !constraint.AdmitsAny(dep.Constraint) {
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithFields(logrus.Fields{
					"name":          pi.Name,
					"version":       pi.Version.Info,
					"depname":       dep.Name,
					"curconstraint": constraint.Body(),
					"newconstraint": dep.Constraint.Body(),
				}).Debug("Project atom cannot be added; its constraints are disjoint with existing constraints")
			}

			// No admissible versions - visit all siblings and identify the disagreement(s)
			for _, sibling := range siblings {
				if !sibling.Dep.Constraint.AdmitsAny(dep.Constraint) {
					if s.l.Level >= logrus.DebugLevel {
						s.l.WithFields(logrus.Fields{
							"name":          pi.Name,
							"version":       pi.Version.Info,
							"depname":       sibling.Depender.Name,
							"sibconstraint": sibling.Dep.Constraint.Body(),
							"newconstraint": dep.Constraint.Body(),
						}).Debug("Marking other, selected project as failed because its constraint is disjoint with our input")
					}
					s.fail(sibling.Depender.Name)
				}
			}

			// TODO msg
			return &disjointConstraintFailure{
				pn:   dep.Name,
				deps: append(siblings, Dependency{Depender: pi, Dep: dep}),
			}
		}

		selected, exists := s.sel.selected(dep.Name)
		if exists && !dep.Constraint.Admits(selected.Version) {
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithFields(logrus.Fields{
					"name":          pi.Name,
					"version":       pi.Version.Info,
					"depname":       dep.Name,
					"curversion":    selected.Version.Info,
					"newconstraint": dep.Constraint.Body(),
				}).Debug("Project atom cannot be added; the constraint it introduces on dep does not allow the currently selected version for that dep")
			}
			s.fail(dep.Name)

			// TODO msg
			return &noVersionError{
				pn:   dep.Name,
				c:    dep.Constraint,
				deps: append(siblings, Dependency{Depender: pi, Dep: dep}),
			}
		}

		// At this point, dart/pub do things related to 'required' dependencies,
		// which is about solving loops (i think) and so mostly not something we
		// have to care about.
	}

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"name":    pi.Name,
			"version": pi.Version.Info,
		}).Debug("Project atom passed satisfiability test against current state")
	}

	return nil
}

// getDependenciesOf returns the dependencies of the given ProjectAtom, mediated
// through any overrides dictated by the root project.
//
// If it's the root project, also includes dev dependencies, etc.
func (s *solver) getDependenciesOf(pi ProjectAtom) ([]ProjectDep, error) {
	info, err := s.sm.GetProjectInfo(pi)
	if err != nil {
		// TODO revisit this once a decision is made about better-formed errors;
		// question is, do we expect the fetcher to pass back simple errors, or
		// well-typed solver errors?
		return nil, err
	}

	deps := info.GetDependencies()
	if s.rp.Name() == pi.Name {
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

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"selcount":   len(s.sel.projects),
			"queuecount": len(s.versions),
			"attempts":   s.attempts,
		}).Debug("Beginning backtracking")
	}

	for {
		for {
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithField("queuecount", len(s.versions)).Debug("Top of search loop for failed queues")
			}

			if len(s.versions) == 0 {
				// no more versions, nowhere further to backtrack
				return false
			}
			if s.versions[len(s.versions)-1].failed {
				break
			}

			if s.l.Level >= logrus.InfoLevel {
				s.l.WithFields(logrus.Fields{
					"name":      s.versions[len(s.versions)-1].ref,
					"wasfailed": false,
				}).Info("Backtracking popped off project")
			}
			// pub asserts here that the last in s.sel's ids is == q.current
			s.versions, s.versions[len(s.versions)-1] = s.versions[:len(s.versions)-1], nil
			s.unselectLast()
		}

		// Grab the last VersionQueue off the list of queues
		q := s.versions[len(s.versions)-1]

		if s.l.Level >= logrus.DebugLevel {
			s.l.WithFields(logrus.Fields{
				"name":    q.ref,
				"failver": q.current().Info,
			}).Debug("Trying failed queue with next version")
		}

		// another assert that the last in s.sel's ids is == q.current
		s.unselectLast()

		// Advance the queue past the current version, which we know is bad
		if q.advance() == nil && !q.isExhausted() {
			// Search for another acceptable version of this failed dep in its queue
			if s.findValidVersion(q) == nil {
				if s.l.Level >= logrus.InfoLevel {
					s.l.WithFields(logrus.Fields{
						"name":    q.ref,
						"version": q.current().Info,
					}).Info("Backtracking found valid version, attempting next solution")
				}

				// Found one! Put it back on the selected queue and stop
				// backtracking
				s.selectVersion(ProjectAtom{
					Name:    q.ref,
					Version: q.current(),
				})
				break
			}
		}

		if s.l.Level >= logrus.DebugLevel {
			s.l.WithFields(logrus.Fields{
				"name": q.ref,
			}).Debug("Failed to find a valid version in queue, continuing backtrack")
		}

		// No solution found; continue backtracking after popping the queue
		// we just inspected off the list
		if s.l.Level >= logrus.InfoLevel {
			s.l.WithFields(logrus.Fields{
				"name":      s.versions[len(s.versions)-1].ref,
				"wasfailed": true,
			}).Info("Backtracking popped off project")
		}
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

func (s *solver) nextUnselected() (ProjectName, bool) {
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

	rname := s.rp.Name()
	// *always* put root project first
	if iname == rname {
		return true
	}
	if jname == rname {
		return false
	}

	ilock, jlock := s.rp.GetProjectAtom(iname) == nil, s.rp.GetProjectAtom(jname) == nil

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

func (s *solver) fail(name ProjectName) {
	// skip if the root project
	if s.rp.Name() == name {
		return
	}

	for _, vq := range s.versions {
		if vq.ref == name {
			vq.failed = true
			// just look for the first (oldest) one; the backtracker will
			// necessarily traverse through and pop off any earlier ones
			// TODO ...right?
			return
		}
	}
}

func (s *solver) selectVersion(pa ProjectAtom) {
	s.unsel.remove(pa.Name)
	s.sel.projects = append(s.sel.projects, pa)

	deps, err := s.getDependenciesOf(pa)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic("shouldn't be possible")
	}

	for _, dep := range deps {
		siblingsAndSelf := append(s.sel.getDependenciesOn(dep.Name), Dependency{Depender: pa, Dep: dep})
		s.sel.deps[dep.Name] = siblingsAndSelf

		// add project to unselected queue if this is the first dep on it -
		// otherwise it's already in there, or been selected
		if len(siblingsAndSelf) == 1 {
			heap.Push(s.unsel, dep.Name)
		}
	}
}

func (s *solver) unselectLast() {
	var pa ProjectAtom
	pa, s.sel.projects = s.sel.projects[len(s.sel.projects)-1], s.sel.projects[:len(s.sel.projects)-1]
	heap.Push(s.unsel, pa.Name)

	deps, err := s.getDependenciesOf(pa)
	if err != nil {
		// if we're choosing a package that has errors getting its deps, there's
		// a bigger problem
		// TODO try to create a test that hits this
		panic("shouldn't be possible")
	}

	for _, dep := range deps {
		siblings := s.sel.getDependenciesOn(dep.Name)
		s.sel.deps[dep.Name] = siblings[:len(siblings)-1]

		// if no siblings, remove from unselected queue
		if len(siblings) == 0 {
			s.unsel.remove(dep.Name)
		}
	}
}
