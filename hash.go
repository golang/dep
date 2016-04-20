package vsolver

import (
	"crypto/sha256"
	"sort"
)

// HashInputs computes a digest of all inputs to a Solve() run.
//
// The digest returned from this function is the same as the digest that would
// be included with the Result be compared against that which is
// returned in a Solve() result - i.e., a lock file. If the digests match, then
// manifest and lock are in sync, and there's no need to Solve().
func (s *solver) HashInputs(path string, m Manifest) []byte {
	d, dd := m.GetDependencies(), m.GetDevDependencies()
	p := make(sortedDeps, len(d))
	copy(p, d)
	p = append(p, dd...)

	sort.Stable(p)

	h := sha256.New()
	for _, pd := range p {
		h.Write([]byte(pd.Name))
		h.Write([]byte(pd.Constraint.String()))
	}

	// TODO static analysis
	// TODO overrides
	// TODO aliases
	// TODO ignores
	return h.Sum(nil)
}

type sortedDeps []ProjectDep

func (s sortedDeps) Len() int {
	return len(s)
}

func (s sortedDeps) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortedDeps) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}
