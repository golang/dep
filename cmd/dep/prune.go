// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/fs"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/pkg/errors"
)

const pruneShortHelp = `Prune the vendor tree of unused packages`
const pruneLongHelp = `
Prune is used to remove unused packages from your vendor tree.

STABILITY NOTICE: this command creates problems for vendor/ verification. As
such, it may be removed and/or moved out into a separate project later on.
`

type pruneCommand struct {
}

func (cmd *pruneCommand) Name() string      { return "prune" }
func (cmd *pruneCommand) Args() string      { return "" }
func (cmd *pruneCommand) ShortHelp() string { return pruneShortHelp }
func (cmd *pruneCommand) LongHelp() string  { return pruneLongHelp }
func (cmd *pruneCommand) Hidden() bool      { return false }

func (cmd *pruneCommand) Register(fs *flag.FlagSet) {
}

func (cmd *pruneCommand) Run(ctx *dep.Ctx, args []string) error {
	p, err := ctx.LoadProject()
	if err != nil {
		return err
	}

	sm, err := ctx.SourceManager()
	if err != nil {
		return err
	}
	sm.UseDefaultSignalHandling()
	defer sm.Release()

	// While the network churns on ListVersions() requests, statically analyze
	// code from the current project.
	ptree, err := pkgtree.ListPackages(p.ResolvedAbsRoot, string(p.ImportRoot))
	if err != nil {
		return errors.Wrap(err, "analysis of local packages failed: %v")
	}

	// Set up a solver in order to check the InputHash.
	params := p.MakeParams()
	params.RootPackageTree = ptree

	if ctx.Verbose {
		params.TraceLogger = ctx.Err
	}

	s, err := gps.Prepare(params, sm)
	if err != nil {
		return errors.Wrap(err, "could not set up solver for input hashing")
	}

	if p.Lock == nil {
		return errors.Errorf("Gopkg.lock must exist for prune to know what files are safe to remove.")
	}

	if !bytes.Equal(s.HashInputs(), p.Lock.SolveMeta.InputsDigest) {
		return errors.Errorf("Gopkg.lock is out of sync; run dep ensure before pruning.")
	}

	var pruneLogger *log.Logger
	if ctx.Verbose {
		pruneLogger = ctx.Err
	}
	return pruneProject(p, sm, pruneLogger)
}

// pruneProject removes unused packages from a project.
func pruneProject(p *dep.Project, sm gps.SourceManager, logger *log.Logger) error {
	td, err := ioutil.TempDir(os.TempDir(), "dep")
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir for writing manifest/lock/vendor")
	}
	defer os.RemoveAll(td)

	if err := gps.WriteDepTree(td, p.Lock, sm, true, logger); err != nil {
		return err
	}

	var toKeep []string
	for _, project := range p.Lock.Projects() {
		projectRoot := string(project.Ident().ProjectRoot)
		for _, pkg := range project.Packages() {
			toKeep = append(toKeep, filepath.Join(projectRoot, pkg))
		}
	}

	toDelete, err := calculatePrune(td, toKeep, logger)
	if err != nil {
		return err
	}

	if logger != nil {
		if len(toDelete) > 0 {
			logger.Println("Calculated the following directories to prune:")
			for _, d := range toDelete {
				logger.Printf("  %s\n", d)
			}
		} else {
			logger.Println("No directories found to prune")
		}
	}

	if err := deleteDirs(toDelete); err != nil {
		return err
	}

	vpath := filepath.Join(p.AbsRoot, "vendor")
	vendorbak := vpath + ".orig"
	var failerr error
	if _, err := os.Stat(vpath); err == nil {
		// Move out the old vendor dir. just do it into an adjacent dir, to
		// try to mitigate the possibility of a pointless cross-filesystem
		// move with a temp directory.
		if _, err := os.Stat(vendorbak); err == nil {
			// If the adjacent dir already exists, bite the bullet and move
			// to a proper tempdir.
			vendorbak = filepath.Join(td, "vendor.orig")
		}
		failerr = fs.RenameWithFallback(vpath, vendorbak)
		if failerr != nil {
			goto fail
		}
	}

	// Move in the new one.
	failerr = fs.RenameWithFallback(td, vpath)
	if failerr != nil {
		goto fail
	}

	os.RemoveAll(vendorbak)

	return nil

fail:
	fs.RenameWithFallback(vendorbak, vpath)
	return failerr
}

func calculatePrune(vendorDir string, keep []string, logger *log.Logger) ([]string, error) {
	if logger != nil {
		logger.Println("Calculating prune. Checking the following packages:")
	}
	sort.Strings(keep)
	toDelete := []string{}
	err := filepath.Walk(vendorDir, func(path string, info os.FileInfo, err error) error {
		if _, err := os.Lstat(path); err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if path == vendorDir {
			return nil
		}

		name := strings.TrimPrefix(path, vendorDir+string(filepath.Separator))
		if logger != nil {
			logger.Printf("  %s", name)
		}
		i := sort.Search(len(keep), func(i int) bool {
			return name <= keep[i]
		})
		if i >= len(keep) || !strings.HasPrefix(keep[i], name) {
			toDelete = append(toDelete, path)
		}
		return nil
	})
	return toDelete, err
}

func deleteDirs(toDelete []string) error {
	// sort by length so we delete sub dirs first
	sort.Sort(byLen(toDelete))
	for _, path := range toDelete {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

type byLen []string

func (a byLen) Len() int           { return len(a) }
func (a byLen) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byLen) Less(i, j int) bool { return len(a[i]) > len(a[j]) }
