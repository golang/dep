package vsolver

import "strings"

type selection struct {
	projects []ProjectID
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
	if !exists {
		return anyConstraint{}
	}

	// TODO recomputing this sucks and is quite wasteful. Precompute/cache it
	// on changes to the constraint set, instead.

	// The solver itself is expected to maintain the invariant that all the
	// constraints kept here collectively admit a non-empty set of versions. We
	// assume this is the case here while assembling a composite constraint.
	//
	// TODO verify that this invariant is maintained; also verify that the slice
	// can't be empty

	// If the first constraint requires an exact match, then we know all the
	// others must be identical, so just return the first one
	if deps[0].Dep.Constraint.Type()&C_ExactMatch != 0 {
		return deps[0].Dep.Constraint
	}

	// Otherwise, we're dealing with semver ranges, so we have to compute the
	// constraint intersection
	var cs []string
	for _, dep := range deps {
		cs = append(cs, dep.Dep.Constraint.Body())
	}

	c, err := NewConstraint(C_SemverRange, strings.Join(cs, ", "))
	if err != nil {
		panic("canary - something wrong with constraint computation")
	}

	return c
}

func (s *selection) selected(id ProjectIdentifier) (ProjectID, bool) {
	for _, pi := range s.projects {
		if pi.ID == id {
			return pi, true
		}
	}

	return ProjectID{}, false
}

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
	//*u.sl = append(*u.sl, x.(ProjectIdentifier))
	u.sl = append(u.sl, x.(ProjectIdentifier))
}

func (u *unselected) Pop() (v interface{}) {
	//old := *u.sl
	//v := old[len(old)-1]
	//*u = old[:len(old)-1]
	v, u.sl = u.sl[len(u.sl)-1], u.sl[:len(u.sl)-1]
	return v
}

// remove takes an ProjectIdentifier out of the priority queue (if it was
// present), then reapplies the heap invariants.
func (u *unselected) remove(id ProjectIdentifier) {
	for k, pi := range u.sl {
		if pi == id {
			u.sl = append(u.sl[:k], u.sl[k+1:]...)
			// TODO need to heap.Fix()? shouldn't have to...
		}
	}
}
