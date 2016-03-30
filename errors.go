package vsolver

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
	pn   ProjectName
	v    string
	c    Constraint
	deps []Dependency
}

func (e *noVersionError) Error() string {
	// TODO compose a message out of the data we have
	return ""
}

type disjointConstraintFailure struct {
	pn   ProjectName
	deps []Dependency
}

func (e *disjointConstraintFailure) Error() string {
	// TODO compose a message out of the data we have
	return ""
}
