package vsolver

type selection struct {
	projects []atomWithPackages
	deps     map[ProjectIdentifier][]Dependency
	sm       sourceBridge
}

func (s *selection) getDependenciesOn(id ProjectIdentifier) []Dependency {
	if deps, exists := s.deps[id]; exists {
		return deps
	}

	return nil
}

func (s *selection) pushDep(dep Dependency) {
	s.deps[dep.Dep.Ident] = append(s.deps[dep.Dep.Ident], dep)
}

func (s *selection) popDep(id ProjectIdentifier) (dep Dependency) {
	deps := s.deps[id]
	dep, s.deps[id] = deps[len(deps)-1], deps[:len(deps)-1]
	return dep
}

func (s *selection) depperCount(id ProjectIdentifier) int {
	return len(s.deps[id])
}

func (s *selection) setDependenciesOn(id ProjectIdentifier, deps []Dependency) {
	s.deps[id] = deps
}

// Compute a unique list of the currently selected packages within a given
// ProjectIdentifier.
func (s *selection) getSelectedPackagesIn(id ProjectIdentifier) map[string]struct{} {
	// TODO this is horribly inefficient to do on the fly; we need a method to
	// precompute it on pushing a new dep, and preferably with an immut
	// structure so that we can pop with zero cost.
	uniq := make(map[string]struct{})
	for _, dep := range s.deps[id] {
		for _, pkg := range dep.Dep.pl {
			uniq[pkg] = struct{}{}
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
		ret = s.sm.intersect(id, ret, dep.Dep.Constraint)
	}

	return ret
}

func (s *selection) selected(id ProjectIdentifier) (atomWithPackages, bool) {
	for _, pi := range s.projects {
		if pi.atom.Ident.eq(id) {
			return pi, true
		}
	}

	return atomWithPackages{atom: nilpa}, false
}

// TODO take a ProjectName, but optionally also a preferred version. This will
// enable the lock files of dependencies to remain slightly more stable.
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
// The worst case for both of these is O(n), but the first case will always
// complete quickly, as we iterate the queue from front to back.
func (u *unselected) remove(bmi bimodalIdentifier) {
	// TODO is it worth implementing a binary search here?
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
