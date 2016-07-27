package gps

import (
	"bytes"
	"fmt"
	"strings"
)

type errorLevel uint8

// TODO(sdboyer) consistent, sensible way of handling 'type' and 'severity' - or figure
// out that they're not orthogonal and collapse into just 'type'

const (
	warning errorLevel = 1 << iota
	mustResolve
	cannotResolve
)

func a2vs(a atom) string {
	if a.v == rootRev || a.v == nil {
		return "(root)"
	}

	return fmt.Sprintf("%s@%s", a.id.errString(), a.v)
}

type traceError interface {
	traceString() string
}

type noVersionError struct {
	pn    ProjectIdentifier
	fails []failedVersion
}

func (e *noVersionError) Error() string {
	if len(e.fails) == 0 {
		return fmt.Sprintf("No versions found for project %q.", e.pn.ProjectRoot)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "No versions of %s met constraints:", e.pn.ProjectRoot)
	for _, f := range e.fails {
		fmt.Fprintf(&buf, "\n\t%s: %s", f.v, f.f.Error())
	}

	return buf.String()
}

func (e *noVersionError) traceString() string {
	if len(e.fails) == 0 {
		return fmt.Sprintf("No versions found")
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "No versions of %s met constraints:", e.pn.ProjectRoot)
	for _, f := range e.fails {
		if te, ok := f.f.(traceError); ok {
			fmt.Fprintf(&buf, "\n  %s: %s", f.v, te.traceString())
		} else {
			fmt.Fprintf(&buf, "\n  %s: %s", f.v, f.f.Error())
		}
	}

	return buf.String()
}

type disjointConstraintFailure struct {
	goal      dependency
	failsib   []dependency
	nofailsib []dependency
	c         Constraint
}

func (e *disjointConstraintFailure) Error() string {
	if len(e.failsib) == 1 {
		str := "Could not introduce %s, as it has a dependency on %s with constraint %s, which has no overlap with existing constraint %s from %s"
		return fmt.Sprintf(str, a2vs(e.goal.depender), e.goal.dep.Ident.errString(), e.goal.dep.Constraint.String(), e.failsib[0].dep.Constraint.String(), a2vs(e.failsib[0].depender))
	}

	var buf bytes.Buffer

	var sibs []dependency
	if len(e.failsib) > 1 {
		sibs = e.failsib

		str := "Could not introduce %s, as it has a dependency on %s with constraint %s, which has no overlap with the following existing constraints:\n"
		fmt.Fprintf(&buf, str, a2vs(e.goal.depender), e.goal.dep.Ident.errString(), e.goal.dep.Constraint.String())
	} else {
		sibs = e.nofailsib

		str := "Could not introduce %s, as it has a dependency on %s with constraint %s, which does not overlap with the intersection of existing constraints from other currently selected packages:\n"
		fmt.Fprintf(&buf, str, a2vs(e.goal.depender), e.goal.dep.Ident.errString(), e.goal.dep.Constraint.String())
	}

	for _, c := range sibs {
		fmt.Fprintf(&buf, "\t%s from %s\n", c.dep.Constraint.String(), a2vs(c.depender))
	}

	return buf.String()
}

func (e *disjointConstraintFailure) traceString() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "constraint %s on %s disjoint with other dependers:\n", e.goal.dep.Constraint.String(), e.goal.dep.Ident.errString())
	for _, f := range e.failsib {
		fmt.Fprintf(
			&buf,
			"%s from %s (no overlap)\n",
			f.dep.Constraint.String(),
			a2vs(f.depender),
		)
	}
	for _, f := range e.nofailsib {
		fmt.Fprintf(
			&buf,
			"%s from %s (some overlap)\n",
			f.dep.Constraint.String(),
			a2vs(f.depender),
		)
	}

	return buf.String()
}

// Indicates that an atom could not be introduced because one of its dep
// constraints does not admit the currently-selected version of the target
// project.
type constraintNotAllowedFailure struct {
	// The dependency with the problematic constraint that could not be
	// introduced.
	goal dependency
	// The (currently selected) version of the target project that was not
	// admissible by the goal dependency.
	v Version
}

func (e *constraintNotAllowedFailure) Error() string {
	return fmt.Sprintf(
		"Could not introduce %s, as it has a dependency on %s with constraint %s, which does not allow the currently selected version of %s",
		a2vs(e.goal.depender),
		e.goal.dep.Ident.errString(),
		e.goal.dep.Constraint,
		e.v,
	)
}

func (e *constraintNotAllowedFailure) traceString() string {
	return fmt.Sprintf(
		"%s depends on %s with %s, but that's already selected at %s",
		a2vs(e.goal.depender),
		e.goal.dep.Ident.ProjectRoot,
		e.goal.dep.Constraint,
		e.v,
	)
}

// versionNotAllowedFailure describes a failure where an atom is rejected
// because its version is not allowed by current constraints.
//
// (This is one of the more straightforward types of failures)
type versionNotAllowedFailure struct {
	// The atom that was rejected by current constraints.
	goal atom
	// The active dependencies that caused the atom to be rejected. Note that
	// this only includes dependencies that actually rejected the atom, which
	// will be at least one, but may not be all the active dependencies on the
	// atom's identifier.
	failparent []dependency
	// The current constraint on the atom's identifier. This is the composite of
	// all active dependencies' constraints.
	c Constraint
}

func (e *versionNotAllowedFailure) Error() string {
	if len(e.failparent) == 1 {
		return fmt.Sprintf(
			"Could not introduce %s, as it is not allowed by constraint %s from project %s.",
			a2vs(e.goal),
			e.failparent[0].dep.Constraint.String(),
			e.failparent[0].depender.id.errString(),
		)
	}

	var buf bytes.Buffer

	fmt.Fprintf(&buf, "Could not introduce %s, as it is not allowed by constraints from the following projects:\n", a2vs(e.goal))

	for _, f := range e.failparent {
		fmt.Fprintf(&buf, "\t%s from %s\n", f.dep.Constraint.String(), a2vs(f.depender))
	}

	return buf.String()
}

func (e *versionNotAllowedFailure) traceString() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%s not allowed by constraint %s:\n", a2vs(e.goal), e.c.String())
	for _, f := range e.failparent {
		fmt.Fprintf(&buf, "  %s from %s\n", f.dep.Constraint.String(), a2vs(f.depender))
	}

	return buf.String()
}

type missingSourceFailure struct {
	goal ProjectIdentifier
	prob string
}

func (e *missingSourceFailure) Error() string {
	return fmt.Sprintf(e.prob, e.goal)
}

type badOptsFailure string

func (e badOptsFailure) Error() string {
	return string(e)
}

type sourceMismatchFailure struct {
	// The ProjectRoot over which there is disagreement about where it should be
	// sourced from
	shared ProjectRoot
	// The current value for the network source
	current string
	// The mismatched value for the network source
	mismatch string
	// The currently selected dependencies which have agreed upon/established
	// the given network source
	sel []dependency
	// The atom with the constraint that has the new, incompatible network source
	prob atom
}

func (e *sourceMismatchFailure) Error() string {
	var cur []string
	for _, c := range e.sel {
		cur = append(cur, string(c.depender.id.ProjectRoot))
	}

	str := "Could not introduce %s, as it depends on %s from %s, but %s is already marked as coming from %s by %s"
	return fmt.Sprintf(str, a2vs(e.prob), e.shared, e.mismatch, e.shared, e.current, strings.Join(cur, ", "))
}

func (e *sourceMismatchFailure) traceString() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "disagreement on network addr for %s:\n", e.shared)

	fmt.Fprintf(&buf, "  %s from %s\n", e.mismatch, e.prob.id.errString())
	for _, dep := range e.sel {
		fmt.Fprintf(&buf, "  %s from %s\n", e.current, dep.depender.id.errString())
	}

	return buf.String()
}

type errDeppers struct {
	err     error
	deppers []atom
}
type checkeeHasProblemPackagesFailure struct {
	goal    atom
	failpkg map[string]errDeppers
}

func (e *checkeeHasProblemPackagesFailure) Error() string {
	var buf bytes.Buffer
	indent := ""

	if len(e.failpkg) > 1 {
		indent = "\t"
		fmt.Fprintf(
			&buf, "Could not introduce %s due to multiple problematic subpackages:\n",
			a2vs(e.goal),
		)
	}

	for pkg, errdep := range e.failpkg {
		var cause string
		if errdep.err == nil {
			cause = "is missing"
		} else {
			cause = fmt.Sprintf("does not contain usable Go code (%T).", errdep.err)
		}

		if len(e.failpkg) == 1 {
			fmt.Fprintf(
				&buf, "Could not introduce %s, as its subpackage %s %s.",
				a2vs(e.goal),
				pkg,
				cause,
			)
		} else {
			fmt.Fprintf(&buf, "\tSubpackage %s %s.", pkg, cause)
		}

		if len(errdep.deppers) == 1 {
			fmt.Fprintf(
				&buf, " (Package is required by %s.)",
				a2vs(errdep.deppers[0]),
			)
		} else {
			fmt.Fprintf(&buf, " Package is required by:")
			for _, pa := range errdep.deppers {
				fmt.Fprintf(&buf, "\n%s\t%s", indent, a2vs(pa))
			}
		}
	}

	return buf.String()
}

func (e *checkeeHasProblemPackagesFailure) traceString() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%s at %s has problem subpkg(s):\n", e.goal.id.ProjectRoot, e.goal.v)
	for pkg, errdep := range e.failpkg {
		if errdep.err == nil {
			fmt.Fprintf(&buf, "\t%s is missing; ", pkg)
		} else {
			fmt.Fprintf(&buf, "\t%s has err (%T); ", pkg, errdep.err)
		}

		if len(errdep.deppers) == 1 {
			fmt.Fprintf(&buf, "required by %s.", a2vs(errdep.deppers[0]))
		} else {
			fmt.Fprintf(&buf, " required by:")
			for _, pa := range errdep.deppers {
				fmt.Fprintf(&buf, "\n\t\t%s at %s", pa.id.errString(), pa.v)
			}
		}
	}

	return buf.String()
}

type depHasProblemPackagesFailure struct {
	goal dependency
	v    Version
	pl   []string
	prob map[string]error
}

func (e *depHasProblemPackagesFailure) Error() string {
	fcause := func(pkg string) string {
		var cause string
		if err, has := e.prob[pkg]; has {
			cause = fmt.Sprintf("does not contain usable Go code (%T).", err)
		} else {
			cause = "is missing."
		}
		return cause
	}

	if len(e.pl) == 1 {
		return fmt.Sprintf(
			"Could not introduce %s, as it requires package %s from %s, but in version %s that package %s",
			a2vs(e.goal.depender),
			e.pl[0],
			e.goal.dep.Ident.errString(),
			e.v,
			fcause(e.pl[0]),
		)
	}

	var buf bytes.Buffer
	fmt.Fprintf(
		&buf, "Could not introduce %s, as it requires problematic packages from %s (current version %s):",
		a2vs(e.goal.depender),
		e.goal.dep.Ident.errString(),
		e.v,
	)

	for _, pkg := range e.pl {
		fmt.Fprintf(&buf, "\t%s %s", pkg, fcause(pkg))
	}

	return buf.String()
}

func (e *depHasProblemPackagesFailure) traceString() string {
	var buf bytes.Buffer
	fcause := func(pkg string) string {
		var cause string
		if err, has := e.prob[pkg]; has {
			cause = fmt.Sprintf("has parsing err (%T).", err)
		} else {
			cause = "is missing"
		}
		return cause
	}

	fmt.Fprintf(
		&buf, "%s depping on %s at %s has problem subpkg(s):",
		a2vs(e.goal.depender),
		e.goal.dep.Ident.errString(),
		e.v,
	)

	for _, pkg := range e.pl {
		fmt.Fprintf(&buf, "\t%s %s", pkg, fcause(pkg))
	}

	return buf.String()
}

// nonexistentRevisionFailure indicates that a revision constraint was specified
// for a given project, but that that revision does not exist in the source
// repository.
type nonexistentRevisionFailure struct {
	goal dependency
	r    Revision
}

func (e *nonexistentRevisionFailure) Error() string {
	return fmt.Sprintf(
		"Could not introduce %s, as it requires %s at revision %s, but that revision does not exist",
		a2vs(e.goal.depender),
		e.goal.dep.Ident.errString(),
		e.r,
	)
}

func (e *nonexistentRevisionFailure) traceString() string {
	return fmt.Sprintf(
		"%s wants missing rev %s of %s",
		a2vs(e.goal.depender),
		e.r,
		e.goal.dep.Ident.errString(),
	)
}
