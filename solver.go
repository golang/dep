package vsolver

import (
	"container/heap"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/Sirupsen/logrus"
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

// SolveOpts holds both options that govern solving behavior, and the actual
// inputs to the solving process.
type SolveOpts struct {
	Root                 string
	N                    ProjectName
	M                    Manifest
	L                    Lock
	Downgrade, ChangeAll bool
	ToChange             []ProjectName
}

func NewSolver(sm SourceManager, l *logrus.Logger) Solver {
	if l == nil {
		l = logrus.New()
	}

	return &solver{
		sm:     &smAdapter{sm: sm},
		l:      l,
		latest: make(map[ProjectName]struct{}),
		rlm:    make(map[ProjectName]LockedProject),
	}
}

// solver is a specialized backtracking SAT solver with satisfiability
// conditions hardcoded to the needs of the Go package management problem space.
type solver struct {
	l        *logrus.Logger
	o        SolveOpts
	sm       *smAdapter
	latest   map[ProjectName]struct{}
	sel      *selection
	unsel    *unselected
	versions []*versionQueue
	rlm      map[ProjectName]LockedProject
	attempts int
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
	s.sm.sortdown = opts.Downgrade
	s.sm.vlists = make(map[ProjectName][]Version)

	s.o = opts

	if s.o.L != nil {
		for _, lp := range s.o.L.Projects() {
			s.rlm[lp.n] = lp
		}
	}

	for _, v := range s.o.ToChange {
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

		if s.l.Level >= logrus.DebugLevel {
			s.l.WithFields(logrus.Fields{
				"attempts": s.attempts,
				"name":     id,
				"selcount": len(s.sel.projects),
			}).Debug("Beginning step in solve loop")
		}

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

		if s.l.Level >= logrus.InfoLevel {
			s.l.WithFields(logrus.Fields{
				"name":    queue.id,
				"version": queue.current(),
			}).Info("Accepted project atom")
		}

		s.selectVersion(ProjectAtom{
			Ident:   queue.id,
			Version: queue.current(),
		})
		s.versions = append(s.versions, queue)
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
	if id.LocalName == s.o.M.Name() {
		return newVersionQueue(id, nilpa, s.sm)
	}

	exists, err := s.sm.repoExists(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		exists, err = s.sm.vendorCodeExists(id)
		if err != nil {
			return nil, err
		}
		if exists {
			// Project exists only in vendor (and in some manifest somewhere)
			// TODO mark this for special handling, somehow?
			if s.l.Level >= logrus.WarnLevel {
				s.l.WithFields(logrus.Fields{
					"name": id,
				}).Warn("Code found in vendor for project, but no history was found upstream or in cache")
			}
		} else {
			if s.l.Level >= logrus.WarnLevel {
				s.l.WithFields(logrus.Fields{
					"name": id,
				}).Warn("Upstream project does not exist")
			}
			return nil, newSolveError(fmt.Sprintf("Project '%s' could not be located.", id), cannotResolve)
		}
	}

	lockv, err := s.getLockVersionIfValid(id)
	if err != nil {
		// Can only get an error here if an upgrade was expressly requested on
		// code that exists only in vendor
		return nil, err
	}

	q, err := newVersionQueue(id, lockv, s.sm)
	if err != nil {
		// TODO this particular err case needs to be improved to be ONLY for cases
		// where there's absolutely nothing findable about a given project name
		if s.l.Level >= logrus.WarnLevel {
			s.l.WithFields(logrus.Fields{
				"name": id,
				"err":  err,
			}).Warn("Failed to create a version queue")
		}
		return nil, err
	}

	if s.l.Level >= logrus.DebugLevel {
		if lockv == nilpa {
			s.l.WithFields(logrus.Fields{
				"name":  id,
				"queue": q,
			}).Debug("Created versionQueue, but no data in lock for project")
		} else {
			s.l.WithFields(logrus.Fields{
				"name":  id,
				"queue": q,
			}).Debug("Created versionQueue using version found in lock")
		}
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

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"name":      q.id.errString(),
			"hasLock":   q.hasLock,
			"allLoaded": q.allLoaded,
			"queue":     q,
		}).Debug("Beginning search through versionQueue for a valid version")
	}
	for {
		cur := q.current()
		err := s.satisfiable(ProjectAtom{
			Ident:   q.id,
			Version: cur,
		})
		if err == nil {
			// we have a good version, can return safely
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithFields(logrus.Fields{
					"name":    q.id.errString(),
					"version": cur,
				}).Debug("Found acceptable version, returning out")
			}
			return nil
		}

		if q.advance(err) != nil {
			// Error on advance, have to bail out
			if s.l.Level >= logrus.WarnLevel {
				s.l.WithFields(logrus.Fields{
					"name": q.id.errString(),
					"err":  err,
				}).Warn("Advancing version queue returned unexpected error, marking project as failed")
			}
			break
		}
		if q.isExhausted() {
			// Queue is empty, bail with error
			if s.l.Level >= logrus.InfoLevel {
				s.l.WithField("name", q.id.errString()).Info("Version queue was completely exhausted, marking project as failed")
			}
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

func (s *solver) getLockVersionIfValid(id ProjectIdentifier) (ProjectAtom, error) {
	// If the project is specifically marked for changes, then don't look for a
	// locked version.
	if _, explicit := s.latest[id.LocalName]; explicit || s.o.ChangeAll {
		// For projects with an upstream or cache repository, it's safe to
		// ignore what's in the lock, because there's presumably more versions
		// to be found and attempted in the repository. If it's only in vendor,
		// though, then we have to try to use what's in the lock, because that's
		// the only version we'll be able to get.
		if exist, _ := s.sm.repoExists(id); exist {
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

	// TODO need to make rlm operate on the full ProjectIdentifier
	lp, exists := s.rlm[id.LocalName]
	if !exists {
		if s.l.Level >= logrus.DebugLevel {
			s.l.WithField("name", id).Debug("Project not present in lock")
		}
		return nilpa, nil
	}

	constraint := s.sel.getConstraint(id)
	if !constraint.Matches(lp.v) {
		if s.l.Level >= logrus.InfoLevel {
			s.l.WithFields(logrus.Fields{
				"name":    id,
				"version": lp.Version(),
			}).Info("Project found in lock, but version not allowed by current constraints")
		}
		return nilpa, nil
	}

	if s.l.Level >= logrus.InfoLevel {
		s.l.WithFields(logrus.Fields{
			"name":    id,
			"version": lp.Version(),
		}).Info("Project found in lock")
	}

	return ProjectAtom{
		Ident:   id,
		Version: lp.Version(),
	}, nil
}

// getDependenciesOf returns the dependencies of the given ProjectAtom, mediated
// through any overrides dictated by the root project.
//
// If it's the root project, also includes dev dependencies, etc.
func (s *solver) getDependenciesOf(pa ProjectAtom) ([]ProjectDep, error) {
	var deps []ProjectDep

	// If we're looking for root's deps, get it from opts rather than sm
	if s.o.M.Name() == pa.Ident.LocalName {
		deps = append(s.o.M.GetDependencies(), s.o.M.GetDevDependencies()...)
	} else {
		info, err := s.sm.getProjectInfo(pa)
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
					"name":      s.versions[len(s.versions)-1].id,
					"wasfailed": false,
				}).Info("Backtracking popped off project")
			}
			// pub asserts here that the last in s.sel's ids is == q.current
			s.versions, s.versions[len(s.versions)-1] = s.versions[:len(s.versions)-1], nil
			s.unselectLast()
		}

		// Grab the last versionQueue off the list of queues
		q := s.versions[len(s.versions)-1]

		if s.l.Level >= logrus.DebugLevel {
			s.l.WithFields(logrus.Fields{
				"name":    q.id.errString(),
				"failver": q.current(),
			}).Debug("Trying failed queue with next version")
		}

		// another assert that the last in s.sel's ids is == q.current
		s.unselectLast()

		// Advance the queue past the current version, which we know is bad
		// TODO is it feasible to make available the failure reason here?
		if q.advance(nil) == nil && !q.isExhausted() {
			// Search for another acceptable version of this failed dep in its queue
			if s.findValidVersion(q) == nil {
				if s.l.Level >= logrus.InfoLevel {
					s.l.WithFields(logrus.Fields{
						"name":    q.id.errString(),
						"version": q.current(),
					}).Info("Backtracking found valid version, attempting next solution")
				}

				// Found one! Put it back on the selected queue and stop
				// backtracking
				s.selectVersion(ProjectAtom{
					Ident:   q.id,
					Version: q.current(),
				})
				break
			}
		}

		if s.l.Level >= logrus.DebugLevel {
			s.l.WithFields(logrus.Fields{
				"name": q.id.errString(),
			}).Debug("Failed to find a valid version in queue, continuing backtrack")
		}

		// No solution found; continue backtracking after popping the queue
		// we just inspected off the list
		if s.l.Level >= logrus.InfoLevel {
			s.l.WithFields(logrus.Fields{
				"name":      s.versions[len(s.versions)-1].id.errString(),
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

	rname := s.o.M.Name()
	// *always* put root project first
	if iname.LocalName == rname {
		return true
	}
	if jname.LocalName == rname {
		return false
	}

	_, ilock := s.rlm[iname.LocalName]
	_, jlock := s.rlm[jname.LocalName]

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
	ivl, _ := s.sm.listVersions(iname)
	jvl, _ := s.sm.listVersions(jname)
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
	if s.o.M.Name() == i.LocalName {
		s.l.Debug("Not marking the root project as failed")
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
		s.sel.deps[dep.Ident] = siblingsAndSelf

		// add project to unselected queue if this is the first dep on it -
		// otherwise it's already in there, or been selected
		if len(siblingsAndSelf) == 1 {
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
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithFields(logrus.Fields{
					"name":  dep.Ident,
					"pname": pa.Ident,
					"pver":  pa.Version,
				}).Debug("Removing project from unselected queue; last parent atom was unselected")
			}
			s.unsel.remove(dep.Ident)
		}
	}
}

// simple (temporary?) helper just to convert atoms into locked projects
func pa2lp(pa ProjectAtom) LockedProject {
	lp := LockedProject{
		n: pa.Ident.LocalName,
		// path is mostly duplicate information now, but if we ever allow
		// nesting as a conflict resolution mechanism, it will become valuable
		path: string(pa.Ident.LocalName),
		uri:  pa.Ident.netName(),
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
