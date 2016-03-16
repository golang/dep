package vsolver

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
