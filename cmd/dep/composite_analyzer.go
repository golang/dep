// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

// compositeAnalyzer overlays configuration from multiple analyzers
type compositeAnalyzer struct {
	// Analyzers is the set of analyzers to apply, last one wins any conflicts
	Analyzers []rootProjectAnalyzer
}

func (a compositeAnalyzer) DeriveRootManifestAndLock(path string, n gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	var rootM *dep.Manifest
	var rootL *dep.Lock

	for _, a := range a.Analyzers {
		m, l, err := a.DeriveRootManifestAndLock(path, n)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "%T.DeriveRootManifestAndLock", a)
		}

		if rootM == nil && rootL == nil {
			rootM = m
			rootL = l
		} else {
			// Overlay the changes from the analyzer on-top of the previous analyzer's work
			if m != nil {
				for pkg, prj := range m.Dependencies {
					rootM.Dependencies[pkg] = prj
				}
				for pkg, prj := range m.Ovr {
					rootM.Ovr[pkg] = prj
				}
				for _, pkg := range m.Required {
					if !contains(rootM.Required, pkg) {
						rootM.Required = append(rootM.Required, pkg)
					}
				}
				for _, pkg := range m.Ignored {
					if !contains(rootM.Ignored, pkg) {
						rootM.Ignored = append(rootM.Ignored, pkg)
					}
				}
			}

			if l != nil {
				for _, lp := range l.P {
					for i, existingLP := range rootL.P {
						if lp.Ident().ProjectRoot == existingLP.Ident().ProjectRoot {
							rootL.P[i] = lp
						}
					}
				}
			}
		}
	}

	return rootM, rootL, nil
}

func (a compositeAnalyzer) FinalizeManifestAndLock(m *dep.Manifest, l *dep.Lock) {
	for _, a := range a.Analyzers {
		a.FinalizeManifestAndLock(m, l)
	}
}
