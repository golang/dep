package vsolver

import (
	"bytes"
	"fmt"
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
	pn    ProjectName
	fails []failedVersion
}

func (e *noVersionError) Error() string {
	if len(e.fails) == 0 {
		return fmt.Sprintf("No versions could be found for project %q.", e.pn)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Could not find any versions of %s that met constraints:\n", e.pn)
	for _, f := range e.fails {
		fmt.Fprintf(&buf, "\t%s: %s", f.v.Info, f.f.Error())
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
		return fmt.Sprintf(str, e.goal.Depender.Name, e.goal.Depender.Version.Info, e.goal.Dep.Name, e.goal.Dep.Constraint.Body(), e.failsib[0].Dep.Constraint.Body(), e.failsib[0].Depender.Name, e.failsib[0].Depender.Version.Info)
	}

	var buf bytes.Buffer

	var sibs []Dependency
	if len(e.failsib) > 1 {
		sibs = e.failsib

		str := "Could not introduce %s at %s, as it has a dependency on %s with constraint %s, which has no overlap with the following existing constraints:\n"
		fmt.Fprintf(&buf, str, e.goal.Depender.Name, e.goal.Depender.Version.Info, e.goal.Dep.Name, e.goal.Dep.Constraint.Body())
	} else {
		sibs = e.nofailsib

		str := "Could not introduce %s at %s, as it has a dependency on %s with constraint %s, which does not overlap with the intersection of existing constraints from other currently selected packages:\n"
		fmt.Fprintf(&buf, str, e.goal.Depender.Name, e.goal.Depender.Version.Info, e.goal.Dep.Name, e.goal.Dep.Constraint.Body())
	}

	for _, c := range sibs {
		fmt.Fprintf(&buf, "\t%s at %s with constraint %s\n", c.Depender.Name, c.Depender.Version.Info, c.Dep.Constraint.Body())
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
	return fmt.Sprintf(str, e.goal.Depender.Name, e.goal.Depender.Version.Info, e.goal.Dep.Name, e.goal.Dep.Constraint, e.v.Info)
}

type versionNotAllowedFailure struct {
	goal       ProjectAtom
	failparent []Dependency
	c          Constraint
}

func (e *versionNotAllowedFailure) Error() string {
	if len(e.failparent) == 1 {
		str := "Could not introduce %s at %s, as it is not allowed by constraint %s from project %s."
		return fmt.Sprintf(str, e.goal.Name, e.goal.Version.Info, e.failparent[0].Dep.Constraint.Body(), e.failparent[0].Depender.Name)
	}

	var buf bytes.Buffer

	str := "Could not introduce %s at %s, as it is not allowed by constraints from the following projects:\n"
	fmt.Fprintf(&buf, str, e.goal.Name, e.goal.Version.Info)

	for _, f := range e.failparent {
		fmt.Fprintf(&buf, "\t%s at %s with constraint %s\n", f.Depender.Name, f.Depender.Version.Info, f.Dep.Constraint.Body())
	}

	return buf.String()
}
