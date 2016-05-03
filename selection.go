package vsolver

type selection struct {
	projects []ProjectAtom
	deps     map[ProjectIdentifier][]Dependency
}

func (s *selection) getDependenciesOn(id ProjectIdentifier) []Dependency {
	if deps, exists := s.deps[id]; exists {
		return deps
	}

	return nil
}

func (s *selection) setDependenciesOn(id ProjectIdentifier, deps []Dependency) {
	s.deps[id] = deps
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
		ret = ret.Intersect(dep.Dep.Constraint)
	}

	return ret
}

func (s *selection) selected(id ProjectIdentifier) (ProjectAtom, bool) {
	for _, pi := range s.projects {
		// TODO do we change this on ProjectAtom too, or not?
		if pi.Name.eq(id) {
			return pi, true
		}
	}

	return nilpa, false
}

// TODO take a ProjectName, but optionally also a preferred version. This will
// enable the lock files of dependencies to remain slightly more stable.
type unselected struct {
	sl  []ProjectIdentifier
	cmp func(i, j int) bool
}

// TODO should these be pointer receivers? container/heap examples aren't
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
	u.sl = append(u.sl, x.(ProjectIdentifier))
}

func (u *unselected) Pop() (v interface{}) {
	v, u.sl = u.sl[len(u.sl)-1], u.sl[:len(u.sl)-1]
	return v
}

// remove takes a ProjectIdentifier out of the priority queue (if it was
// present), then reasserts the heap invariants.
func (u *unselected) remove(id ProjectIdentifier) {
	for k, pi := range u.sl {
		if pi == id {
			if k == len(u.sl)-1 {
				// if we're on the last element, just pop, no splice
				u.sl = u.sl[:len(u.sl)-1]
			} else {
				u.sl = append(u.sl[:k], u.sl[k+1:]...)
			}
			break
			// TODO need to heap.Fix()? shouldn't have to...
		}
	}
}
