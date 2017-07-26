// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
func WriteDepTree(basedir string, l Lock, sm SourceManager, sv bool) error {
	if l == nil {
		return fmt.Errorf("must provide non-nil Lock to WriteDepTree")
	}

	err := os.MkdirAll(basedir, 0777)
	if err != nil {
		return err
	}

	wp := newWorkerPool(l.Projects(), 2)

	err = wp.writeDepTree(
		func(p LockedProject) error {
			to := filepath.FromSlash(filepath.Join(basedir, string(p.Ident().ProjectRoot)))

			if err := sm.ExportProject(p.Ident(), p.Version(), to); err != nil {
				return err
			}

			if sv {
				filepath.Walk(to, stripVendor)
			}

			// TODO(sdboyer) dump version metadata file

			return nil
		},
	)

	if err != nil {
		removeAll(basedir)
		return err
	}

	return nil
}

func newWorkerPool(pjs []LockedProject, wc int) *workerPool {
	wp := &workerPool{
		workChan:    make(chan LockedProject),
		doneChan:    make(chan done),
		projects:    pjs,
		workerCount: wc,
	}

	wp.spawnWorkers()

	return wp
}

type workerPool struct {
	workChan    chan LockedProject
	doneChan    chan done
	projects    []LockedProject
	wpFunc      writeProjectFunc
	workerCount int
}

type done struct {
	root ProjectRoot
	err  error
}

type writeProjectFunc func(p LockedProject) error

func (wp *workerPool) spawnWorkers() {
	for i := 0; i < wp.workerCount; i++ {
		go func() {
			for p := range wp.workChan {
				wp.doneChan <- done{p.Ident().ProjectRoot, wp.wpFunc(p)}
			}
		}()
	}
}

func (wp *workerPool) writeDepTree(f writeProjectFunc) error {
	wp.wpFunc = f
	go wp.dispatchJobs()
	return wp.reportProgress()
}

func (wp *workerPool) dispatchJobs() {
	defer close(wp.workChan)
	for _, p := range wp.projects {
		wp.workChan <- p
	}
}

func (wp *workerPool) reportProgress() error {
	var errs []string

	for i := 0; i < len(wp.projects); i++ {
		d := <-wp.doneChan
		if d.err != nil {
			errs = append(errs, fmt.Sprintf("error while exporting %s: %s", d.root, d.err))
		}
	}

	if errs != nil {
		return fmt.Errorf(strings.Join(errs, "\n"))
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
