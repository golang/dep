package vsolver

import "github.com/Sirupsen/logrus"

// satisfiable is the main checking method - it determines if introducing a new
// project atom would result in a graph where all requirements are still
// satisfied.
func (s *solver) satisfiable(pa ProjectAtom) error {
	if emptyProjectAtom == pa {
		// TODO we should protect against this case elsewhere, but for now panic
		// to canary when it's a problem
		panic("canary - checking version of empty ProjectAtom")
	}

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"name":    pa.Name,
			"version": pa.Version,
		}).Debug("Checking satisfiability of project atom against current constraints")
	}

	if err := s.checkAtomAllowable(pa); err != nil {
		return err
	}

	deps, err := s.getDependenciesOf(pa)
	if err != nil {
		// An err here would be from the package fetcher; pass it straight back
		return err
	}

	for _, dep := range deps {
		// TODO dart skips "magic" deps here; do we need that?
		if err := s.checkDepsConstraintsAllowable(pa, dep); err != nil {
			return err
		}
		if err := s.checkDepsDisallowsSelected(pa, dep); err != nil {
			return err
		}

		// TODO add check that fails if adding this atom would create a loop
	}

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"name":    pa.Name,
			"version": pa.Version,
		}).Debug("Project atom passed satisfiability test against current state")
	}

	return nil
}

// checkAtomAllowable ensures that an atom itself is acceptable with respect to
// the constraints established by the current solution.
func (s *solver) checkAtomAllowable(pa ProjectAtom) error {
	constraint := s.sel.getConstraint(pa.Name)
	if constraint.Matches(pa.Version) {
		return nil
	}
	// TODO collect constraint failure reason

	if s.l.Level >= logrus.InfoLevel {
		s.l.WithFields(logrus.Fields{
			"name":          pa.Name,
			"version":       pa.Version,
			"curconstraint": constraint.String(),
		}).Info("Current constraints do not allow version")
	}

	deps := s.sel.getDependenciesOn(pa.Name)
	var failparent []Dependency
	for _, dep := range deps {
		if !dep.Dep.Constraint.Matches(pa.Version) {
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithFields(logrus.Fields{
					"name":       pa.Name,
					"othername":  dep.Depender.Name,
					"constraint": dep.Dep.Constraint.String(),
				}).Debug("Marking other, selected project with conflicting constraint as failed")
			}
			s.fail(dep.Depender.Name)
			failparent = append(failparent, dep)
		}
	}

	return &versionNotAllowedFailure{
		goal:       pa,
		failparent: failparent,
		c:          constraint,
	}
}

// checkDepsConstraintsAllowable checks that the constraints of an atom on a
// given dep would not result in UNSAT.
func (s *solver) checkDepsConstraintsAllowable(pa ProjectAtom, dep ProjectDep) error {
	constraint := s.sel.getConstraint(dep.Ident)
	// Ensure the constraint expressed by the dep has at least some possible
	// intersection with the intersection of existing constraints.
	if constraint.MatchesAny(dep.Constraint) {
		return nil
	}

	if s.l.Level >= logrus.DebugLevel {
		s.l.WithFields(logrus.Fields{
			"name":          pa.Name,
			"version":       pa.Version,
			"depname":       dep.Ident,
			"curconstraint": constraint.String(),
			"newconstraint": dep.Constraint.String(),
		}).Debug("Project atom cannot be added; its constraints are disjoint with existing constraints")
	}

	siblings := s.sel.getDependenciesOn(dep.Ident)
	// No admissible versions - visit all siblings and identify the disagreement(s)
	var failsib []Dependency
	var nofailsib []Dependency
	for _, sibling := range siblings {
		if !sibling.Dep.Constraint.MatchesAny(dep.Constraint) {
			if s.l.Level >= logrus.DebugLevel {
				s.l.WithFields(logrus.Fields{
					"name":          pa.Name,
					"version":       pa.Version,
					"depname":       sibling.Depender.Name,
					"sibconstraint": sibling.Dep.Constraint.String(),
					"newconstraint": dep.Constraint.String(),
				}).Debug("Marking other, selected project as failed because its constraint is disjoint with our testee")
			}
			s.fail(sibling.Depender.Name)
			failsib = append(failsib, sibling)
		} else {
			nofailsib = append(nofailsib, sibling)
		}
	}

	return &disjointConstraintFailure{
		goal:      Dependency{Depender: pa, Dep: dep},
		failsib:   failsib,
		nofailsib: nofailsib,
		c:         constraint,
	}
}

// checkDepsDisallowsSelected ensures that an atom's constraints on a particular
// dep are not incompatible with the version of that dep that's already been
// selected.
func (s *solver) checkDepsDisallowsSelected(pa ProjectAtom, dep ProjectDep) error {
	selected, exists := s.sel.selected(dep.Ident)
	if exists && !dep.Constraint.Matches(selected.Version) {
		if s.l.Level >= logrus.DebugLevel {
			s.l.WithFields(logrus.Fields{
				"name":          pa.Name,
				"version":       pa.Version,
				"depname":       dep.Ident,
				"curversion":    selected.Version,
				"newconstraint": dep.Constraint.String(),
			}).Debug("Project atom cannot be added; a constraint it introduces does not allow a currently selected version")
		}
		s.fail(dep.Ident)

		return &constraintNotAllowedFailure{
			goal: Dependency{Depender: pa, Dep: dep},
			v:    selected.Version,
		}
	}
	return nil
}
