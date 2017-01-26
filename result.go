package gps

import (
	"fmt"
	"os"
	"path/filepath"
)

// A Solution is returned by a solver run. It is mostly just a Lock, with some
// additional methods that report information about the solve run.
type Solution interface {
	Lock
	Attempts() int
}

type solution struct {
	// A list of the projects selected by the solver.
	p []LockedProject

	// The number of solutions that were attempted
	att int

	// The hash digest of the input opts
	hd []byte
}

// WriteDepTree takes a basedir and a Lock, and exports all the projects
// listed in the lock to the appropriate target location within the basedir.
//
// If the goal is to populate a vendor directory, basedir should be the absolute
// path to that vendor directory, not its parent (a project root, typically).
//
// It requires a SourceManager to do the work, and takes a flag indicating
// whether or not to strip vendor directories contained in the exported
// dependencies.
func WriteDepTree(basedir string, l Lock, sm SourceManager, sv bool) error {
	if l == nil {
		return fmt.Errorf("must provide non-nil Lock to WriteDepTree")
	}

	err := os.MkdirAll(basedir, 0777)
	if err != nil {
		return err
	}

	// TODO(sdboyer) parallelize
	for _, p := range l.Projects() {
		to := filepath.FromSlash(filepath.Join(basedir, string(p.Ident().ProjectRoot)))

		err = sm.ExportProject(p.Ident(), p.Version(), to)
		if err != nil {
			removeAll(basedir)
			return fmt.Errorf("error while exporting %s: %s", p.Ident().ProjectRoot, err)
		}
		if sv {
			filepath.Walk(to, stripVendor)
		}
		// TODO(sdboyer) dump version metadata file
	}

	return nil
}

func (r solution) Projects() []LockedProject {
	return r.p
}

func (r solution) Attempts() int {
	return r.att
}

func (r solution) InputHash() []byte {
	return r.hd
}
