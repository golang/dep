package vsolver

// checkProject performs all constraint checks on a new project (with packages)
// that we want to select. It determines if selecting the atom would result in
// a state where all solver requirements are still satisfied.
func (s *solver) checkProject(a atomWithPackages) error {
	pa := a.atom
	if nilpa == pa {
		// This shouldn't be able to happen, but if it does, it unequivocally
		// indicates a logical bug somewhere, so blowing up is preferable
		panic("canary - checking version of empty ProjectAtom")
	}

	if err := s.checkAtomAllowable(pa); err != nil {
		return err
	}

	if err := s.checkRequiredPackagesExist(a); err != nil {
		return err
	}

	deps, err := s.getImportsAndConstraintsOf(a)
	if err != nil {
		// An err here would be from the package fetcher; pass it straight back
		return err
	}

	for _, dep := range deps {
		if err := s.checkIdentMatches(a, dep); err != nil {
			return err
		}
		if err := s.checkDepsConstraintsAllowable(a, dep); err != nil {
			return err
		}
		if err := s.checkDepsDisallowsSelected(a, dep); err != nil {
			return err
		}

		// TODO add check that fails if adding this atom would create a loop
	}

	return nil
}

// checkPackages performs all constraint checks new packages being added to an
// already-selected project. It determines if selecting the packages would
// result in a state where all solver requirements are still satisfied.
func (s *solver) checkPackage(a atomWithPackages) error {
	if nilpa == a.atom {
		// This shouldn't be able to happen, but if it does, it unequivocally
		// indicates a logical bug somewhere, so blowing up is preferable
		panic("canary - checking version of empty ProjectAtom")
	}

	// The base atom was already validated, so we can skip the
	// checkAtomAllowable step.
	deps, err := s.getImportsAndConstraintsOf(a)
	if err != nil {
		// An err here would be from the package fetcher; pass it straight back
		return err
	}

	for _, dep := range deps {
		if err := s.checkIdentMatches(a, dep); err != nil {
			return err
		}
		if err := s.checkDepsConstraintsAllowable(a, dep); err != nil {
			return err
		}
		if err := s.checkDepsDisallowsSelected(a, dep); err != nil {
			return err
		}
	}

	return nil
}

// checkAtomAllowable ensures that an atom itself is acceptable with respect to
// the constraints established by the current solution.
func (s *solver) checkAtomAllowable(pa ProjectAtom) error {
	constraint := s.sel.getConstraint(pa.Ident)
	if s.b.matches(pa.Ident, constraint, pa.Version) {
		return nil
	}
	// TODO collect constraint failure reason (wait...aren't we, below?)

	deps := s.sel.getDependenciesOn(pa.Ident)
	var failparent []Dependency
	for _, dep := range deps {
		if !s.b.matches(pa.Ident, dep.Dep.Constraint, pa.Version) {
			s.fail(dep.Depender.Ident)
			failparent = append(failparent, dep)
		}
	}

	err := &versionNotAllowedFailure{
		goal:       pa,
		failparent: failparent,
		c:          constraint,
	}

	s.logSolve(err)
	return err
}

// checkRequiredPackagesExist ensures that all required packages enumerated by
// existing dependencies on this atom are actually present in the atom.
func (s *solver) checkRequiredPackagesExist(a atomWithPackages) error {
	ptree, err := s.b.listPackages(a.atom.Ident, a.atom.Version)
	if err != nil {
		// TODO handle this more gracefully
		return err
	}

	deps := s.sel.getDependenciesOn(a.atom.Ident)
	fp := make(map[string]errDeppers)
	// We inspect these in a bit of a roundabout way, in order to incrementally
	// build up the failure we'd return if there is, indeed, a missing package.
	// TODO rechecking all of these every time is wasteful. Is there a shortcut?
	for _, dep := range deps {
		for _, pkg := range dep.Dep.pl {
			if errdep, seen := fp[pkg]; seen {
				errdep.deppers = append(errdep.deppers, dep.Depender)
				fp[pkg] = errdep
			} else {
				perr, has := ptree.Packages[pkg]
				if !has || perr.Err != nil {
					fp[pkg] = errDeppers{
						err:     perr.Err,
						deppers: []ProjectAtom{dep.Depender},
					}
				}
			}
		}
	}

	if len(fp) > 0 {
		return &checkeeHasProblemPackagesFailure{
			goal:    a.atom,
			failpkg: fp,
		}
	}
	return nil
}

// checkDepsConstraintsAllowable checks that the constraints of an atom on a
// given dep are valid with respect to existing constraints.
func (s *solver) checkDepsConstraintsAllowable(a atomWithPackages, cdep completeDep) error {
	dep := cdep.ProjectDep
	constraint := s.sel.getConstraint(dep.Ident)
	// Ensure the constraint expressed by the dep has at least some possible
	// intersection with the intersection of existing constraints.
	if s.b.matchesAny(dep.Ident, constraint, dep.Constraint) {
		return nil
	}

	siblings := s.sel.getDependenciesOn(dep.Ident)
	// No admissible versions - visit all siblings and identify the disagreement(s)
	var failsib []Dependency
	var nofailsib []Dependency
	for _, sibling := range siblings {
		if !s.b.matchesAny(dep.Ident, sibling.Dep.Constraint, dep.Constraint) {
			s.fail(sibling.Depender.Ident)
			failsib = append(failsib, sibling)
		} else {
			nofailsib = append(nofailsib, sibling)
		}
	}

	err := &disjointConstraintFailure{
		goal:      Dependency{Depender: a.atom, Dep: cdep},
		failsib:   failsib,
		nofailsib: nofailsib,
		c:         constraint,
	}
	s.logSolve(err)
	return err
}

// checkDepsDisallowsSelected ensures that an atom's constraints on a particular
// dep are not incompatible with the version of that dep that's already been
// selected.
func (s *solver) checkDepsDisallowsSelected(a atomWithPackages, cdep completeDep) error {
	dep := cdep.ProjectDep
	selected, exists := s.sel.selected(dep.Ident)
	if exists && !s.b.matches(dep.Ident, dep.Constraint, selected.atom.Version) {
		s.fail(dep.Ident)

		err := &constraintNotAllowedFailure{
			goal: Dependency{Depender: a.atom, Dep: cdep},
			v:    selected.atom.Version,
		}
		s.logSolve(err)
		return err
	}
	return nil
}

// checkIdentMatches ensures that the LocalName of a dep introduced by an atom,
// has the same NetworkName as what's already been selected (assuming anything's
// been selected).
//
// In other words, this ensures that the solver never simultaneously selects two
// identifiers with the same local name, but that disagree about where their
// network source is.
func (s *solver) checkIdentMatches(a atomWithPackages, cdep completeDep) error {
	dep := cdep.ProjectDep
	if cur, exists := s.names[dep.Ident.LocalName]; exists {
		if cur != dep.Ident.netName() {
			deps := s.sel.getDependenciesOn(a.atom.Ident)
			// Fail all the other deps, as there's no way atom can ever be
			// compatible with them
			for _, d := range deps {
				s.fail(d.Depender.Ident)
			}

			err := &sourceMismatchFailure{
				shared:   dep.Ident.LocalName,
				sel:      deps,
				current:  cur,
				mismatch: dep.Ident.netName(),
				prob:     a.atom,
			}
			s.logSolve(err)
			return err
		}
	}

	return nil
}
