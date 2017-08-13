// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"log"
	"os"
	"path/filepath"
	"sync"

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

// WriteDepTree takes a baseDir and a Lock, and exports all the projects
// listed in the lock to the appropriate target location within baseDir.
//
// If the goal is to populate a vendor directory, baseDir should be the absolute
// path to that vendor directory, not its parent (a project root, typically).
//
// It requires a SourceManager to do the work, and takes a PruneOptions
// indicating the pruning options required for the exported dependencies.
func WriteDepTree(baseDir string, l Lock, sm SourceManager, prune PruneOptions, logger *log.Logger) error {
	if baseDir == "" {
		return errors.New("must provide a non-empty baseDir")
	}
	if l == nil {
		return errors.New("must provide a non-nil Lock to WriteDepTree")
	}
	if sm == nil {
		return errors.New("must provide a non-nil SourceManager to WriteDepTree")
	}

	if err := os.MkdirAll(baseDir, 0777); err != nil {
		return err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(l.Projects()))

	for _, p := range l.Projects() {
		wg.Add(1)
		go func(p LockedProject) {
			to := filepath.FromSlash(filepath.Join(baseDir, string(p.Ident().ProjectRoot)))

			logger.Printf("Writing out %s@%s", p.Ident().errString(), p.Version())

			if err := sm.ExportProject(p.Ident(), p.Version(), to); err != nil {
				removeAll(to)
				errCh <- errors.Wrapf(err, "failed to export %s", p.Ident().ProjectRoot)
			}

			wg.Done()
		}(p)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		logger.Println("Failed to write dep tree. The following errors occurred:")
		for err := range errCh {
			logger.Println(" * ", err)
		}
		removeAll(baseDir)
		// TODO(ibrasho) handle multiple errors
		return <-errCh
	}

	if err := Prune(baseDir, prune, l, logger); err != nil {
		logger.Println("Failed to prune dep tree.")
		removeAll(baseDir)
		return err
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
