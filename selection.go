package vsolver

type selection struct {
	projects []ProjectIdentifier
	deps     map[ProjectIdentifier][]Dependency
}

func (s *selection) nextUnselected() ProjectIdentifier {
	if len(s.projects) > 0 {
		return s.projects[0]
	}
	// TODO ...should actually pop off the list?
	return ""
}

func (s *selection) getDependenciesOn(id ProjectIdentifier) []Dependency {
	return s.deps[id]
}

func (s *selection) setDependenciesOn(id ProjectIdentifier, deps []Dependency) {
	s.deps[id] = deps
}

func (s *selection) getConstraint(id ProjectIdentifier) Constraint {

}

type ProjectIdentifierQueueItem struct {
	ident []byte
	index int
}

//type unselected []*ProjectIdentifierQueueItem
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
