// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

type godepFile struct {
	json    godepJson
	loggers *dep.Loggers
}

func newGodepFile(loggers *dep.Loggers) *godepFile {
	return &godepFile{loggers: loggers}
}

type godepJson struct {
	Name    string         `json:"ImportPath"`
	Imports []godepPackage `json:"Deps"`
}

type godepPackage struct {
	ImportPath string `json:"ImportPath"`
	Rev        string `json:"Rev"`
	Comment    string `json:"Comment"`
}

// load parses Godeps.json in projectDir and unmarshals the json to godepFile.json
func (g *godepFile) load(projectDir string) error {
	j := filepath.Join(projectDir, "Godeps", godepJsonName)
	if g.loggers.Verbose {
		g.loggers.Err.Printf("godep: Loading %s", j)
	}

	raw, err := ioutil.ReadFile(j)
	if err != nil {
		return errors.Wrapf(err, "Unable to read %s", j)
	}
	err = json.Unmarshal(raw, &g.json)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse %s", j)
	}

	return nil
}

func (g *godepFile) convert(projectName string, sm gps.SourceManager) (*dep.Manifest, *dep.Lock, error) {
	// Create empty manifest and lock
	manifest := &dep.Manifest{
		Dependencies: make(gps.ProjectConstraints),
	}
	lock := &dep.Lock{}

	// Parse through each import and add them to manifest and lock
	for _, pkg := range g.json.Imports {
		// ImportPath must not be empty
		if pkg.ImportPath == "" {
			err := errors.New("godep: Invalid godep configuration, ImportPath is required")
			return nil, nil, err
		}

		// Rev must not be empty
		if pkg.Rev == "" {
			err := errors.New("godep: Invalid godep configuration, Rev is required")
			return nil, nil, err
		}

		if pkg.Comment == "" {
			// When there's no comment, try to get corresponding version for the Rev
			// and fill Comment.
			// Get all the versions
			pi := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.ImportPath)}
			versions, err := sm.ListVersions(pi)
			if err != nil {
				return nil, nil, err
			}
			// Sort the versions in descending order, newer versions first
			gps.SortPairedForUpgrade(versions)
			// Match Rev with versions' underlying revision
			for _, v := range versions {
				if string(v.Underlying()) == pkg.Rev {
					pkg.Comment = v.String()
					break
				}
			}
		}

		if pkg.Comment != "" {
			// If there's a comment, use it to create project constraint
			pc, err := g.buildProjectConstraint(pkg, sm)
			if err != nil {
				return nil, nil, err
			}
			manifest.Dependencies[pc.Ident.ProjectRoot] = gps.ProjectProperties{Source: pc.Ident.Source, Constraint: pc.Constraint}
		}

		// Use the revision and comment to create lock project
		lp := g.buildLockedProject(pkg, manifest)
		lock.P = append(lock.P, lp)
	}

	return manifest, lock, nil
}

func (g *godepFile) buildProjectConstraint(pkg godepPackage, sm gps.SourceManager) (pc gps.ProjectConstraint, err error) {
	pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.ImportPath)}
	pc.Constraint, err = deduceConstraint(pkg.Comment, pc.Ident, sm)
	return
}

func (g *godepFile) buildLockedProject(pkg godepPackage, manifest *dep.Manifest) gps.LockedProject {
	var ver gps.Version
	id := gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.ImportPath)}
	c, has := manifest.Dependencies[id.ProjectRoot]
	if has {
		// Create PairedVersion if constraint details are available
		version := gps.NewVersion(c.Constraint.String())
		ver = version.Is(gps.Revision(pkg.Rev))
	} else {
		ver = gps.Revision(pkg.Rev)
	}

	return gps.NewLockedProject(id, ver, nil)
}
