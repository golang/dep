package vsolver

import (
	"bytes"
	"fmt"
	"strings"
)

type errorLevel uint8

// TODO consistent, sensible way of handling 'type' and 'severity' - or figure
// out that they're not orthogonal and collapse into just 'type'

const (
	warning errorLevel = 1 << iota
	mustResolve
	cannotResolve
)

type SolveError interface {
	error
	Children() []error
}

type traceError interface {
	traceString() string
}

type solveError struct {
	lvl errorLevel
	msg string
}

func newSolveError(msg string, lvl errorLevel) error {
	return &solveError{msg: msg, lvl: lvl}
}

func (e *solveError) Error() string {
	return e.msg
}

type noVersionError struct {
	pn    ProjectIdentifier
	fails []failedVersion
}

func (e *noVersionError) Error() string {
	if len(e.fails) == 0 {
		return fmt.Sprintf("No versions could be found for project %q.", e.pn)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Could not find any versions of %s that met constraints:", e.pn)
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
	fmt.Fprintf(&buf, "No versions of %s met constraints:", e.pn.LocalName)
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
	goal      Dependency
	failsib   []Dependency
	nofailsib []Dependency
	c         Constraint
}

func (e *disjointConstraintFailure) Error() string {
	if len(e.failsib) == 1 {
		str := "Could not introduce %s at %s, as it has a dependency on %s with constraint %s, which has no overlap with existing constraint %s from %s at %s"
		return fmt.Sprintf(str, e.goal.Depender.Ident.errString(), e.goal.Depender.Version, e.goal.Dep.Ident.errString(), e.goal.Dep.Constraint.String(), e.failsib[0].Dep.Constraint.String(), e.failsib[0].Depender.Ident.errString(), e.failsib[0].Depender.Version)
	}

	var buf bytes.Buffer

	var sibs []Dependency
	if len(e.failsib) > 1 {
		sibs = e.failsib

		str := "Could not introduce %s at %s, as it has a dependency on %s with constraint %s, which has no overlap with the following existing constraints:\n"
		fmt.Fprintf(&buf, str, e.goal.Depender.Ident.errString(), e.goal.Depender.Version, e.goal.Dep.Ident.errString(), e.goal.Dep.Constraint.String())
	} else {
		sibs = e.nofailsib

		str := "Could not introduce %s at %s, as it has a dependency on %s with constraint %s, which does not overlap with the intersection of existing constraints from other currently selected packages:\n"
		fmt.Fprintf(&buf, str, e.goal.Depender.Ident.errString(), e.goal.Depender.Version, e.goal.Dep.Ident.errString(), e.goal.Dep.Constraint.String())
	}

	for _, c := range sibs {
		fmt.Fprintf(&buf, "\t%s from %s at %s\n", c.Dep.Constraint.String(), c.Depender.Ident.errString(), c.Depender.Version)
	}

	return buf.String()
}

func (e *disjointConstraintFailure) traceString() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "constraint %s on %s disjoint with other dependers:\n", e.goal.Dep.Constraint.String(), e.goal.Dep.Ident.errString())
	for _, f := range e.failsib {
		fmt.Fprintf(&buf, "%s from %s at %s (no overlap)\n", f.Dep.Constraint.String(), f.Depender.Ident.LocalName, f.Depender.Version)
	}
	for _, f := range e.nofailsib {
		fmt.Fprintf(&buf, "%s from %s at %s (some overlap)\n", f.Dep.Constraint.String(), f.Depender.Ident.LocalName, f.Depender.Version)
	}

	return buf.String()
}

// Indicates that an atom could not be introduced because one of its dep
// constraints does not admit the currently-selected version of the target
// project.
type constraintNotAllowedFailure struct {
	goal Dependency
	v    Version
}

func (e *constraintNotAllowedFailure) Error() string {
	str := "Could not introduce %s at %s, as it has a dependency on %s with constraint %s, which does not allow the currently selected version of %s"
	return fmt.Sprintf(str, e.goal.Depender.Ident.errString(), e.goal.Depender.Version, e.goal.Dep.Ident.errString(), e.goal.Dep.Constraint, e.v)
}

func (e *constraintNotAllowedFailure) traceString() string {
	str := "%s at %s depends on %s with %s, but that's already selected at %s"
	return fmt.Sprintf(str, e.goal.Depender.Ident.LocalName, e.goal.Depender.Version, e.goal.Dep.Ident.LocalName, e.goal.Dep.Constraint, e.v)
}

type versionNotAllowedFailure struct {
	goal       ProjectAtom
	failparent []Dependency
	c          Constraint
}

func (e *versionNotAllowedFailure) Error() string {
	if len(e.failparent) == 1 {
		str := "Could not introduce %s at %s, as it is not allowed by constraint %s from project %s."
		return fmt.Sprintf(str, e.goal.Ident.errString(), e.goal.Version, e.failparent[0].Dep.Constraint.String(), e.failparent[0].Depender.Ident.errString())
	}

	var buf bytes.Buffer

	str := "Could not introduce %s at %s, as it is not allowed by constraints from the following projects:\n"
	fmt.Fprintf(&buf, str, e.goal.Ident.errString(), e.goal.Version)

	for _, f := range e.failparent {
		fmt.Fprintf(&buf, "\t%s from %s at %s\n", f.Dep.Constraint.String(), f.Depender.Ident.errString(), f.Depender.Version)
	}

	return buf.String()
}

func (e *versionNotAllowedFailure) traceString() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%s at %s not allowed by constraint %s:\n", e.goal.Ident.LocalName, e.goal.Version, e.c.String())
	for _, f := range e.failparent {
		fmt.Fprintf(&buf, "  %s from %s at %s\n", f.Dep.Constraint.String(), f.Depender.Ident.LocalName, f.Depender.Version)
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

type BadOptsFailure string

func (e BadOptsFailure) Error() string {
	return string(e)
}

type sourceMismatchFailure struct {
	shared            ProjectName
	sel               []Dependency
	current, mismatch string
	prob              ProjectAtom
}

func (e *sourceMismatchFailure) Error() string {
	var cur []string
	for _, c := range e.sel {
		cur = append(cur, string(c.Depender.Ident.LocalName))
	}

	str := "Could not introduce %s at %s, as it depends on %s from %s, but %s is already marked as coming from %s by %s"
	return fmt.Sprintf(str, e.prob.Ident.errString(), e.prob.Version, e.shared, e.mismatch, e.shared, e.current, strings.Join(cur, ", "))
}

func (e *sourceMismatchFailure) traceString() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "disagreement on network addr for %s:\n", e.shared)

	fmt.Fprintf(&buf, "  %s from %s\n", e.mismatch, e.prob.Ident.errString())
	for _, dep := range e.sel {
		fmt.Fprintf(&buf, "  %s from %s\n", e.current, dep.Depender.Ident.errString())
	}

	return buf.String()
}
