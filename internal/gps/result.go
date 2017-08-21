// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// A Solution is returned by a solver run. It is mostly just a Lock, with some
// additional methods that report information about the solve run.
type Solution interface {
	Lock
	// The name of the ProjectAnalyzer used in generating this solution.
	AnalyzerName() string
	// The version of the ProjectAnalyzer used in generating this solution.
	AnalyzerVersion() int
	// The name of the Solver used in generating this solution.
	SolverName() string
	// The version of the Solver used in generating this solution.
	SolverVersion() int
	Attempts() int
}

type solution struct {
	// A list of the projects selected by the solver.
	p []LockedProject

	// The number of solutions that were attempted
	att int

	// The hash digest of the input opts
	hd []byte

	// The analyzer info
	analyzerInfo ProjectAnalyzerInfo

	// The solver used in producing this solution
	solv Solver
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
func WriteDepTree(basedir string, l Lock, sm SourceManager, sv bool, logger *log.Logger) error {
	if l == nil {
		return fmt.Errorf("must provide non-nil Lock to WriteDepTree")
	}

	err := os.MkdirAll(basedir, 0777)
	if err != nil {
		return err
	}

	lps := l.Projects()

	type resp struct {
		i   int
		err error
	}
	respCh := make(chan resp, len(lps))

	for i := range lps {
		go func(i int) {
			p := lps[i]

			var err error
			defer func() {
				if r := recover(); r != nil {
					err = errors.Errorf("recovered from panic exporting %s: %s", p.Ident().ProjectRoot, r)
				}
				respCh <- resp{i, err}
			}()

			to := filepath.FromSlash(filepath.Join(basedir, string(p.Ident().ProjectRoot)))

			if err = sm.ExportProject(p.Ident(), p.Version(), to); err != nil {
				err = errors.Wrapf(err, "failed to export %s", p.Ident().ProjectRoot)
				return
			}

			if sv {
				err = filepath.Walk(to, stripVendor)
				if err != nil {
					err = errors.Wrapf(err, "failed to strip vendor from %s", p.Ident().ProjectRoot)
				}
			}
		}(i)
	}

	var errs []error
	for i := 0; i < len(lps); i++ {
		resp := <-respCh
		msg := "Wrote"
		if resp.err != nil {
			errs = append(errs, resp.err)
			msg = "Failed to write"
		}
		p := lps[resp.i]
		logger.Printf("(%d/%d) %s %s@%s\n", i+1, len(lps), msg, p.Ident(), p.Version())
	}
	close(respCh)

	if len(errs) > 0 {
		logger.Println("Failed to write dep tree. The following errors occurred:")
		for i, err := range errs {
			logger.Printf("(%d/%d) %s\n", i+1, len(errs), err)
		}

		removeAll(basedir)

		return errors.New("failed to write dep tree")
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

func (r solution) AnalyzerName() string {
	return r.analyzerInfo.Name
}

func (r solution) AnalyzerVersion() int {
	return r.analyzerInfo.Version
}

func (r solution) SolverName() string {
	return r.solv.Name()
}

func (r solution) SolverVersion() int {
	return r.solv.Version()
}
