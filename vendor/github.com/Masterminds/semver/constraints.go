package semver

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var constraintRegex *regexp.Regexp
var constraintRangeRegex *regexp.Regexp

const cvRegex string = `v?([0-9|x|X|\*]+)(\.[0-9|x|X|\*]+)?(\.[0-9|x|X|\*]+)?` +
	`(-([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?` +
	`(\+([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?`

func init() {
	constraintOps := []string{
		"",
		"=",
		"!=",
		">",
		"<",
		">=",
		"=>",
		"<=",
		"=<",
		"~",
		"~>",
		"^",
	}

	ops := make([]string, 0, len(constraintOps))
	for _, op := range constraintOps {
		ops = append(ops, regexp.QuoteMeta(op))
	}

	constraintRegex = regexp.MustCompile(fmt.Sprintf(
		`^\s*(%s)\s*(%s)\s*$`,
		strings.Join(ops, "|"),
		cvRegex))

	constraintRangeRegex = regexp.MustCompile(fmt.Sprintf(
		`\s*(%s)\s*-\s*(%s)\s*`,
		cvRegex, cvRegex))
}

type Constraint interface {
	// Constraints compose the fmt.Stringer interface. Printing a constraint
	// will yield a string that, if passed to NewConstraint(), will produce the
	// original constraint. (Bidirectional serialization)
	fmt.Stringer

	// Admits checks that a version satisfies the constraint. If it does not,
	// an error is returned indcating the problem; if it does, the error is nil.
	Admits(v *Version) error

	// Intersect computes the intersection between the receiving Constraint and
	// passed Constraint, and returns a new Constraint representing the result.
	Intersect(Constraint) Constraint

	// Union computes the union between the receiving Constraint and the passed
	// Constraint, and returns a new Constraint representing the result.
	Union(Constraint) Constraint

	// AdmitsAny returns a bool indicating whether there exists any version that
	// satisfies both the receiver constraint, and the passed Constraint.
	//
	// In other words, this reports whether an intersection would be non-empty.
	AdmitsAny(Constraint) bool

	// Restrict implementation of this interface to this package. We need the
	// flexibility of an interface, but we cover all possibilities here; closing
	// off the interface to external implementation lets us safely do tricks
	// with types for magic types (none and any)
	_private()
}

// realConstraint is used internally to differentiate between any, none, and
// unionConstraints, vs. Version and rangeConstraints.
type realConstraint interface {
	Constraint
	_real()
}

// Controls whether or not parsed constraints are cached
var cacheConstraints = true
var constraintCache = make(map[string]Constraint)

// NewConstraint takes a string representing a set of semver constraints, and
// returns a corresponding Constraint object. Constraints are suitable
// for checking Versions for admissibility, or combining with other Constraint
// objects.
//
// If an invalid constraint string is passed, more information is provided in
// the returned error string.
func NewConstraint(in string) (Constraint, error) {
	if cacheConstraints {
		// This means reparsing errors, but oh well
		if final, exists := constraintCache[in]; exists {
			return final, nil
		}
	}

	// Rewrite - ranges into a comparison operation.
	c := rewriteRange(in)

	ors := strings.Split(c, "||")
	or := make([]Constraint, len(ors))
	for k, v := range ors {
		cs := strings.Split(v, ",")
		result := make([]Constraint, len(cs))
		for i, s := range cs {
			pc, err := parseConstraint(s)
			if err != nil {
				return nil, err
			}

			result[i] = pc
		}
		or[k] = Intersection(result...)
	}

	final := Union(or...)
	if cacheConstraints {
		constraintCache[in] = final
	}

	return final, nil
}

// Intersection computes the intersection between N Constraints, returning as
// compact a representation of the intersection as possible.
//
// No error is indicated if all the sets are collectively disjoint; you must inspect the
// return value to see if the result is the empty set (indicated by both
// IsMagic() being true, and AdmitsAny() being false).
func Intersection(cg ...Constraint) Constraint {
	// If there's zero or one constraints in the group, we can quit fast
	switch len(cg) {
	case 0:
		// Zero members, only sane thing to do is return none
		return None()
	case 1:
		// Just one member means that's our final constraint
		return cg[0]
	}

	// Preliminary first pass to look for a none (that would supercede everything
	// else), and also construct a []realConstraint for everything else
	var real constraintList

	for _, c := range cg {
		switch tc := c.(type) {
		case any:
			continue
		case none:
			return c
		case *Version:
			real = append(real, tc)
		case rangeConstraint:
			real = append(real, tc)
		case unionConstraint:
			real = append(real, tc...)
		default:
			panic("unknown constraint type")
		}
	}

	sort.Sort(real)

	// Now we know there's no easy wins, so step through and intersect each with
	// the previous
	car, cdr := cg[0], cg[1:]
	for _, c := range cdr {
		car = car.Intersect(c)
		if IsNone(car) {
			return None()
		}
	}

	return car
}

// Union takes a variable number of constraints, and returns the most compact
// possible representation of those constraints.
//
// This effectively ORs together all the provided constraints. If any of the
// included constraints are the set of all versions (any), that supercedes
// everything else.
func Union(cg ...Constraint) Constraint {
	// If there's zero or one constraints in the group, we can quit fast
	switch len(cg) {
	case 0:
		// Zero members, only sane thing to do is return none
		return None()
	case 1:
		// One member, so the result will just be that
		return cg[0]
	}

	// Preliminary pass to look for 'any' in the current set (and bail out early
	// if found), but also construct a []realConstraint for everything else
	var real constraintList

	for _, c := range cg {
		switch tc := c.(type) {
		case any:
			return c
		case none:
			continue
		case *Version:
			real = append(real, tc)
		case rangeConstraint:
			real = append(real, tc)
		case unionConstraint:
			real = append(real, tc...)
		default:
			panic("unknown constraint type")
		}
	}

	// Sort both the versions and ranges into ascending order
	sort.Sort(real)

	// Iteratively merge the constraintList elements
	var nuc unionConstraint
	for _, c := range real {
		if len(nuc) == 0 {
			nuc = append(nuc, c)
			continue
		}

		last := nuc[len(nuc)-1]
		if last.AdmitsAny(c) || areAdjacent(last, c) {
			nuc[len(nuc)-1] = last.Union(c).(realConstraint)
		} else {
			nuc = append(nuc, c)
		}
	}

	if len(nuc) == 1 {
		return nuc[0]
	}
	return nuc
}

type ascendingRanges []rangeConstraint

func (rs ascendingRanges) Len() int {
	return len(rs)
}

func (rs ascendingRanges) Less(i, j int) bool {
	ir, jr := rs[i].max, rs[j].max
	inil, jnil := ir == nil, jr == nil

	if !inil && !jnil {
		if ir.LessThan(jr) {
			return true
		}
		if jr.LessThan(ir) {
			return false
		}

		// Last possible - if i is inclusive, but j isn't, then put i after j
		if !rs[j].includeMax && rs[i].includeMax {
			return false
		}

		// Or, if j inclusive, but i isn't...but actually, since we can't return
		// 0 on this comparator, this handles both that and the 'stable' case
		return true
	} else if inil || jnil {
		// ascending, so, if jnil, then j has no max but i does, so i should
		// come first. thus, return jnil
		return jnil
	}

	// neither have maxes, so now go by the lowest min
	ir, jr = rs[i].min, rs[j].min
	inil, jnil = ir == nil, jr == nil

	if !inil && !jnil {
		if ir.LessThan(jr) {
			return true
		}
		if jr.LessThan(ir) {
			return false
		}

		// Last possible - if j is inclusive, but i isn't, then put i after j
		if rs[j].includeMin && !rs[i].includeMin {
			return false
		}

		// Or, if i inclusive, but j isn't...but actually, since we can't return
		// 0 on this comparator, this handles both that and the 'stable' case
		return true
	} else if inil || jnil {
		// ascending, so, if inil, then i has no min but j does, so j should
		// come first. thus, return inil
		return inil
	}

	// Default to keeping i before j
	return true
}

func (rs ascendingRanges) Swap(i, j int) {
	rs[i], rs[j] = rs[j], rs[i]
}
