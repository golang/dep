package vsolver

import (
	"fmt"
	"os"
	"path"
)

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

func (r Result) CreateVendorTree(basedir string, sm SourceManager) error {
	if r.SolveFailure != nil {
		return fmt.Errorf("Cannot create vendor tree from failed solution. Failure was %s", r.SolveFailure)
	}

	err := os.MkdirAll(basedir, 0777)
	if err != nil {
		return err
	}

	// TODO parallelize
	for _, p := range r.Projects {
		to := path.Join(basedir, string(p.Name))
		os.MkdirAll(to, 0777)
		err := sm.ExportAtomTo(p, to)
		if err != nil {
			os.RemoveAll(basedir)
			return err
		}
		// TODO dump version metadata file
	}

	return nil
}
