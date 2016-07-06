package vsolver

type selection struct {
	projects []selected
	deps     map[ProjectIdentifier][]dependency
	sm       sourceBridge
}

type selected struct {
	a     atomWithPackages
	first bool
}

func (s *selection) getDependenciesOn(id ProjectIdentifier) []dependency {
	if deps, exists := s.deps[id]; exists {
		return deps
	}

	return nil
}

// pushSelection pushes a new atomWithPackages onto the selection stack, along
// with an indicator as to whether this selection indicates a new project *and*
// packages, or merely some new packages on a project that was already selected.
func (s *selection) pushSelection(a atomWithPackages, first bool) {
	s.projects = append(s.projects, selected{
		a:     a,
		first: first,
	})
}

// popSelection removes and returns the last atomWithPackages from the selection
// stack, along with an indication of whether that element was the first from
// that project - that is, if it represented an addition of both a project and
// one or more packages to the overall selection.
func (s *selection) popSelection() (atomWithPackages, bool) {
	var sel selected
	sel, s.projects = s.projects[len(s.projects)-1], s.projects[:len(s.projects)-1]
	return sel.a, sel.first
}

func (s *selection) pushDep(dep dependency) {
	s.deps[dep.dep.Ident] = append(s.deps[dep.dep.Ident], dep)
}

func (s *selection) popDep(id ProjectIdentifier) (dep dependency) {
	deps := s.deps[id]
	dep, s.deps[id] = deps[len(deps)-1], deps[:len(deps)-1]
	return dep
}

func (s *selection) depperCount(id ProjectIdentifier) int {
	return len(s.deps[id])
}

func (s *selection) setDependenciesOn(id ProjectIdentifier, deps []dependency) {
	s.deps[id] = deps
}

// Compute a list of the unique packages within the given ProjectIdentifier that
// have dependers, and the number of dependers they have.
func (s *selection) getRequiredPackagesIn(id ProjectIdentifier) map[string]int {
	// TODO this is horribly inefficient to do on the fly; we need a method to
	// precompute it on pushing a new dep, and preferably with an immut
	// structure so that we can pop with zero cost.
	uniq := make(map[string]int)
	for _, dep := range s.deps[id] {
		for _, pkg := range dep.dep.pl {
			if count, has := uniq[pkg]; has {
				count++
				uniq[pkg] = count
			} else {
				uniq[pkg] = 1
			}
		}
	}

	return uniq
}

// Compute a list of the unique packages within the given ProjectIdentifier that
// are currently selected, and the number of times each package has been
// independently selected.
func (s *selection) getSelectedPackagesIn(id ProjectIdentifier) map[string]int {
	// TODO this is horribly inefficient to do on the fly; we need a method to
	// precompute it on pushing a new dep, and preferably with an immut
	// structure so that we can pop with zero cost.
	uniq := make(map[string]int)
	for _, p := range s.projects {
		if p.a.a.id.eq(id) {
			for _, pkg := range p.a.pl {
				if count, has := uniq[pkg]; has {
					count++
					uniq[pkg] = count
				} else {
					uniq[pkg] = 1
				}
			}
		}
	}

	return uniq
}

func (s *selection) getConstraint(id ProjectIdentifier) Constraint {
	deps, exists := s.deps[id]
	if !exists || len(deps) == 0 {
		return any
	}

	// TODO recomputing this sucks and is quite wasteful. Precompute/cache it
	// on changes to the constraint set, instead.

	// The solver itself is expected to maintain the invariant that all the
	// constraints kept here collectively admit a non-empty set of versions. We
	// assume this is the case here while assembling a composite constraint.

	// Start with the open set
	var ret Constraint = any
	for _, dep := range deps {
		ret = s.sm.intersect(id, ret, dep.dep.Constraint)
	}

	return ret
}

// selected checks to see if the given ProjectIdentifier has been selected, and
// if so, returns the corresponding atomWithPackages.
//
// It walks the projects selection list from front to back and returns the first
// match it finds, which means it will always and only return the base selection
// of the project, without any additional package selections that may or may not
// have happened later.
func (s *selection) selected(id ProjectIdentifier) (atomWithPackages, bool) {
	for _, p := range s.projects {
		if p.a.a.id.eq(id) {
			return p.a, true
		}
	}

	return atomWithPackages{a: nilpa}, false
}

type unselected struct {
	sl  []bimodalIdentifier
	cmp func(i, j int) bool
}

func (u unselected) Len() int {
	return len(u.sl)
}

func (u unselected) Less(i, j int) bool {
	return u.cmp(i, j)
}

func (u unselected) Swap(i, j int) {
	u.sl[i], u.sl[j] = u.sl[j], u.sl[i]
}

func (u *unselected) Push(x interface{}) {
	u.sl = append(u.sl, x.(bimodalIdentifier))
}

func (u *unselected) Pop() (v interface{}) {
	v, u.sl = u.sl[len(u.sl)-1], u.sl[:len(u.sl)-1]
	return v
}

// remove takes a ProjectIdentifier out of the priority queue, if present.
//
// There are, generally, two ways this gets called: to remove the unselected
// item from the front of the queue while that item is being unselected, and
// during backtracking, when an item becomes unnecessary because the item that
// induced it was popped off.
//
// The worst case for both of these is O(n), but in practice the first case is
// be O(1), as we iterate the queue from front to back.
func (u *unselected) remove(bmi bimodalIdentifier) {
	for k, pi := range u.sl {
		if pi.id.eq(bmi.id) {
			// Simple slice comparison - assume they're both sorted the same
			for k, pkg := range pi.pl {
				if bmi.pl[k] != pkg {
					break
				}
			}

			if k == len(u.sl)-1 {
				// if we're on the last element, just pop, no splice
				u.sl = u.sl[:len(u.sl)-1]
			} else {
				u.sl = append(u.sl[:k], u.sl[k+1:]...)
			}
			break
		}
	}
}
