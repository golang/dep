package vsolver

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

type Result interface {
	Lock
	Attempts() int
}

type result struct {
	// A list of the projects selected by the solver.
	p []LockedProject

	// The number of solutions that were attempted
	att int

	// The hash digest of the input opts
	hd []byte
}

// CreateVendorTree takes a basedir and a Lock, and exports all the projects
// listed in the lock to the appropriate target location within the basedir.
//
// It requires a SourceManager to do the work, and takes a flag indicating
// whether or not to strip vendor directories contained in the exported
// dependencies.
func CreateVendorTree(basedir string, l Lock, sm SourceManager, sv bool) error {
	err := os.MkdirAll(basedir, 0777)
	if err != nil {
		return err
	}

	// TODO parallelize
	for _, p := range l.Projects() {
		to := path.Join(basedir, string(p.Ident().LocalName))

		err := os.MkdirAll(to, 0777)
		if err != nil {
			return err
		}

		err = sm.ExportProject(p.Ident().LocalName, p.Version(), to)
		if err != nil {
			removeAll(basedir)
			return fmt.Errorf("Error while exporting %s: %s", p.Ident().LocalName, err)
		}
		if sv {
			filepath.Walk(to, stripVendor)
		}
		// TODO dump version metadata file
	}

	return nil
}

func (r result) Projects() []LockedProject {
	return r.p
}

func (r result) Attempts() int {
	return r.att
}

func (r result) InputHash() []byte {
	return r.hd
}
