package vsolver

type Result struct {
	// A list of the projects selected by the solver. nil if solving failed.
	Projects []ProjectAtom

	// The number of solutions that were attempted
	Attempts int

	// The error that ultimately prevented reaching a successful conclusion. nil
	// if solving was successful.
	// TODO proper error types
	SolveFailure error
}
