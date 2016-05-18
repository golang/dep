package vsolver

import (
	"fmt"
	"os"
	"path"
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

func CreateVendorTree(basedir string, l Lock, sm SourceManager) error {
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

		err = sm.ExportAtomTo(p.toAtom(), to)
		if err != nil {
			os.RemoveAll(basedir)
			return fmt.Errorf("Error while exporting %s: %s", p.Ident().LocalName, err)
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
