package semver

import (
	"fmt"
	"sort"
	"strings"
)

type rangeConstraint struct {
	min, max               *Version
	includeMin, includeMax bool
	excl                   []*Version
}

func (rc rangeConstraint) Matches(v *Version) error {
	var fail bool

	rce := RangeMatchFailure{
		v:  v,
		rc: rc,
	}

	if rc.min != nil {
		// TODO ensure sane handling of prerelease versions (which are strictly
		// less than the normal version, but should be admitted in a geq range)
		cmp := rc.min.Compare(v)
		if rc.includeMin {
			rce.typ = rerrLT
			fail = cmp == 1
		} else {
			rce.typ = rerrLTE
			fail = cmp != -1
		}

		if fail {
			return rce
		}
	}

	if rc.max != nil {
		// TODO ensure sane handling of prerelease versions (which are strictly
		// less than the normal version, but should be admitted in a geq range)
		cmp := rc.max.Compare(v)
		if rc.includeMax {
			rce.typ = rerrGT
			fail = cmp == -1
		} else {
			rce.typ = rerrGTE
			fail = cmp != 1
		}

		if fail {
			return rce
		}
	}

	for _, excl := range rc.excl {
		if excl.Equal(v) {
			rce.typ = rerrNE
			return rce
		}
	}

	return nil
}

func (rc rangeConstraint) dup() rangeConstraint {
	// Only need to do anything if there are some excludes
	if len(rc.excl) == 0 {
		return rc
	}

	var excl []*Version
	excl = make([]*Version, len(rc.excl))
	copy(excl, rc.excl)

	return rangeConstraint{
		min:        rc.min,
		max:        rc.max,
		includeMin: rc.includeMin,
		includeMax: rc.includeMax,
		excl:       excl,
	}
}

func (rc rangeConstraint) Intersect(c Constraint) Constraint {
	switch oc := c.(type) {
	case any:
		return rc
	case none:
		return None()
	case unionConstraint:
		return oc.Intersect(rc)
	case *Version:
		if err := rc.Matches(oc); err != nil {
			return None()
		} else {
			return c
		}
	case rangeConstraint:
		nr := rangeConstraint{
			min:        rc.min,
			max:        rc.max,
			includeMin: rc.includeMin,
			includeMax: rc.includeMax,
		}

		if oc.min != nil {
			if nr.min == nil || nr.min.LessThan(oc.min) {
				nr.min = oc.min
				nr.includeMin = oc.includeMin
			} else if oc.min.Equal(nr.min) && !oc.includeMin {
				// intersection means we must follow the least inclusive
				nr.includeMin = false
			}
		}

		if oc.max != nil {
			if nr.max == nil || nr.max.GreaterThan(oc.max) {
				nr.max = oc.max
				nr.includeMax = oc.includeMax
			} else if oc.max.Equal(nr.max) && !oc.includeMax {
				// intersection means we must follow the least inclusive
				nr.includeMax = false
			}
		}

		// Ensure any applicable excls from oc are included in nc
		for _, e := range append(rc.excl, oc.excl...) {
			if nr.Matches(e) == nil {
				nr.excl = append(nr.excl, e)
			}
		}

		if nr.min == nil || nr.max == nil {
			return nr
		}

		if nr.min.Equal(nr.max) {
			// min and max are equal. if range is inclusive, return that
			// version; otherwise, none
			if nr.includeMin && nr.includeMax {
				return nr.min
			}
			return None()
		}

		if nr.min.GreaterThan(nr.max) {
			// min is greater than max - not possible, so we return none
			return None()
		}

		// range now fully validated, return what we have
		return nr

	default:
		panic("unknown type")
	}
}

func (rc rangeConstraint) Union(c Constraint) Constraint {
	switch oc := c.(type) {
	case any:
		return Any()
	case none:
		return rc
	case unionConstraint:
		return Union(rc, oc)
	case *Version:
		if err := rc.Matches(oc); err == nil {
			return rc
		} else if len(rc.excl) > 0 { // TODO (re)checking like this is wasteful
			// ensure we don't have an excl-specific mismatch; if we do, remove
			// it and return that
			for k, e := range rc.excl {
				if e.Equal(oc) {
					excl := make([]*Version, len(rc.excl)-1)

					if k == len(rc.excl)-1 {
						copy(excl, rc.excl[:k])
					} else {
						copy(excl, append(rc.excl[:k], rc.excl[k+1:]...))
					}

					return rangeConstraint{
						min:        rc.min,
						max:        rc.max,
						includeMin: rc.includeMin,
						includeMax: rc.includeMax,
						excl:       excl,
					}
				}
			}
		}

		if oc.LessThan(rc.min) {
			return unionConstraint{oc, rc.dup()}
		}
		if areEq(oc, rc.min) {
			ret := rc.dup()
			ret.includeMin = true
			return ret
		}
		if areEq(oc, rc.max) {
			ret := rc.dup()
			ret.includeMax = true
			return ret
		}
		// Only possibility left is gt
		return unionConstraint{rc.dup(), oc}
	case rangeConstraint:
		if (rc.min == nil && oc.max == nil) || (rc.max == nil && oc.min == nil) {
			rcl, ocl := len(rc.excl), len(oc.excl)
			// Quick check for open case
			if rcl == 0 && ocl == 0 {
				return Any()
			}

			// This is inefficient, but it's such an absurdly corner case...
			if len(dedupeExcls(rc.excl, oc.excl)) == rcl+ocl {
				// If deduped excludes are the same length as the individual
				// excludes, then they have no overlapping elements, so the
				// union knocks out the excludes and we're back to Any.
				return Any()
			}

			// There's at least some dupes, which are all we need to include
			nc := rangeConstraint{}
			for _, e1 := range rc.excl {
				for _, e2 := range oc.excl {
					if e1.Equal(e2) {
						nc.excl = append(nc.excl, e1)
					}
				}
			}

			return nc
		} else if areAdjacent(rc, oc) {
			// Receiver adjoins the input from below
			nc := rc.dup()

			nc.max = oc.max
			nc.includeMax = oc.includeMax
			nc.excl = append(nc.excl, oc.excl...)

			return nc
		} else if areAdjacent(oc, rc) {
			// Input adjoins the receiver from below
			nc := oc.dup()

			nc.max = rc.max
			nc.includeMax = rc.includeMax
			nc.excl = append(nc.excl, rc.excl...)

			return nc

		} else if rc.MatchesAny(oc) {
			// Receiver and input overlap; form a new range accordingly.
			nc := rangeConstraint{}

			// For efficiency, we simultaneously determine if either of the
			// ranges are supersets of the other, while also selecting the min
			// and max of the new range
			var info uint8

			const (
				lminlt uint8             = 1 << iota // left (rc) min less than right
				rminlt                               // right (oc) min less than left
				lmaxgt                               // left max greater than right
				rmaxgt                               // right max greater than left
				lsupr  = lminlt | lmaxgt             // left is superset of right
				rsupl  = rminlt | rmaxgt             // right is superset of left
			)

			// Pick the min
			if rc.min != nil {
				if oc.min == nil || rc.min.GreaterThan(oc.min) || (rc.min.Equal(oc.min) && !rc.includeMin && oc.includeMin) {
					info |= rminlt
					nc.min = oc.min
					nc.includeMin = oc.includeMin
				} else {
					info |= lminlt
					nc.min = rc.min
					nc.includeMin = rc.includeMin
				}
			} else if oc.min != nil {
				info |= lminlt
				nc.min = rc.min
				nc.includeMin = rc.includeMin
			}

			// Pick the max
			if rc.max != nil {
				if oc.max == nil || rc.max.LessThan(oc.max) || (rc.max.Equal(oc.max) && !rc.includeMax && oc.includeMax) {
					info |= rmaxgt
					nc.max = oc.max
					nc.includeMax = oc.includeMax
				} else {
					info |= lmaxgt
					nc.max = rc.max
					nc.includeMax = rc.includeMax
				}
			} else if oc.max != nil {
				info |= lmaxgt
				nc.max = rc.max
				nc.includeMax = rc.includeMax
			}

			// Reincorporate any excluded versions
			if info&lsupr != lsupr {
				// rc is not superset of oc, so must walk oc.excl
				for _, e := range oc.excl {
					if rc.Matches(e) != nil {
						nc.excl = append(nc.excl, e)
					}
				}
			}

			if info&rsupl != rsupl {
				// oc is not superset of rc, so must walk rc.excl
				for _, e := range rc.excl {
					if oc.Matches(e) != nil {
						nc.excl = append(nc.excl, e)
					}
				}
			}

			return nc
		} else {
			// Don't call Union() here b/c it would duplicate work
			uc := constraintList{rc, oc}
			sort.Sort(uc)
			return unionConstraint(uc)
		}
	}

	panic("unknown type")
}

// isSupersetOf computes whether the receiver rangeConstraint is a superset of
// the passed rangeConstraint.
//
// This is NOT a strict superset comparison, so identical ranges will both
// report being supersets of each other.
//
// Note also that this does *not* compare excluded versions - it only compares
// range endpoints.
func (rc rangeConstraint) isSupersetOf(rc2 rangeConstraint) bool {
	if rc.min != nil {
		if rc2.min == nil || rc.min.GreaterThan(rc2.min) || (rc.min.Equal(rc2.min) && !rc.includeMin && rc2.includeMin) {
			return false
		}
	}

	if rc.max != nil {
		if rc2.max == nil || rc.max.LessThan(rc2.max) || (rc.max.Equal(rc2.max) && !rc.includeMax && rc2.includeMax) {
			return false
		}
	}

	return true
}

func (rc rangeConstraint) String() string {
	// TODO express using caret or tilde, where applicable
	var pieces []string
	if rc.min != nil {
		if rc.includeMin {
			pieces = append(pieces, fmt.Sprintf(">=%s", rc.min))
		} else {
			pieces = append(pieces, fmt.Sprintf(">%s", rc.min))
		}
	}

	if rc.max != nil {
		if rc.includeMax {
			pieces = append(pieces, fmt.Sprintf("<=%s", rc.max))
		} else {
			pieces = append(pieces, fmt.Sprintf("<%s", rc.max))
		}
	}

	for _, e := range rc.excl {
		pieces = append(pieces, fmt.Sprintf("!=%s", e))
	}

	return strings.Join(pieces, ", ")
}

// areAdjacent tests two constraints to determine if they are adjacent,
// but non-overlapping.
//
// If either constraint is not a range, returns false. We still allow it at the
// type level, however, to make the check convenient elsewhere.
//
// Assumes the first range is less than the second; it is incumbent on the
// caller to arrange the inputs appropriately.
func areAdjacent(c1, c2 Constraint) bool {
	var rc1, rc2 rangeConstraint
	var ok bool
	if rc1, ok = c1.(rangeConstraint); !ok {
		return false
	}
	if rc2, ok = c2.(rangeConstraint); !ok {
		return false
	}

	if !areEq(rc1.max, rc2.min) {
		return false
	}

	return (rc1.includeMax && !rc2.includeMin) ||
		(!rc1.includeMax && rc2.includeMin)
}

func (rc rangeConstraint) MatchesAny(c Constraint) bool {
	if _, ok := rc.Intersect(c).(none); ok {
		return false
	}
	return true
}

func dedupeExcls(ex1, ex2 []*Version) []*Version {
	// TODO stupid inefficient, but these are really only ever going to be
	// small, so not worth optimizing right now
	var ret []*Version
oloop:
	for _, e1 := range ex1 {
		for _, e2 := range ex2 {
			if e1.Equal(e2) {
				continue oloop
			}
		}
		ret = append(ret, e1)
	}

	return append(ret, ex2...)
}

func (rangeConstraint) _private() {}
func (rangeConstraint) _real()    {}

func areEq(v1, v2 *Version) bool {
	if v1 == nil && v2 == nil {
		return true
	}

	if v1 != nil && v2 != nil {
		return v1.Equal(v2)
	}
	return false
}
