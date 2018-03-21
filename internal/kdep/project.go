/*
 * Copyright 2018 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package kdep

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/gps/pkgtree"
	"github.com/golang/dep/internal/dependencies"
)

// Project wraps dep.Project to support kdep projects
type Project struct {
	*dep.Project
	Manifest    *Manifest
	SubProjects []*dep.Project

	extraVendorEntries map[string]string
}

func newProject(p *dep.Project, c *Ctx) (*Project, error) {
	m := manifestFromProject(p)
	if m == nil || !m.Meta.IsKdepRoot {
		return nil, fmt.Errorf("not a kdep project")
	}

	sps := make([]*dep.Project, len(m.Meta.LocalDeps))
	sms := make(map[string]*dep.Manifest)
	extra := make(map[string]string)

	for i, sub := range m.Meta.LocalDeps {
		for _, path := range m.Meta.LocalGopaths {
			candidate := filepath.Join(p.ResolvedAbsRoot, path, "src", sub)
			if _, err := os.Stat(candidate); err == nil {
				ctxt := *c.Ctx
				ctxt.WorkingDir = candidate
				proj, err := ctxt.LoadProject()
				if err != nil {
					return nil, err
				}

				// make sure we have the proper import name
				proj.ImportRoot = gps.ProjectRoot(sub)
				sps[i] = proj
				imp := string(proj.ImportRoot)
				sms[imp] = proj.Manifest

				// no need to look further in gopaths
				extra[imp] = candidate
				break
			}
		}
	}
	res := &Project{p, m, sps, extra}
	m.SubManifests = sms

	b := &dependencies.DepsBuilder{
		Root:          p.AbsRoot,
		Package:       string(p.ImportRoot),
		LocalPackages: m.Meta.LocalDeps,
		SkipSubdirs:   m.Meta.LocalGopaths,
	}
	deps, err := b.GetPackageDependencies()
	if err != nil {
		return nil, err
	}

	m.Dependencies = deps

	return res, nil
}

// MakeParams generates resolution parameters
func (p *Project) MakeParams() gps.SolveParameters {
	params := gps.SolveParameters{
		RootDir:         p.AbsRoot,
		ProjectAnalyzer: dep.Analyzer{},
	}

	params.Manifest = p.Manifest

	if p.Lock != nil {
		params.Lock = p.Lock
	}

	return params
}

// ParseRootPackageTree generates the pkgtree.PackageTree for a kdep multi-repo
func (p *Project) ParseRootPackageTree() (pkgtree.PackageTree, error) {
	tree, err := p.Project.ParseRootPackageTree()
	if err != nil {
		return tree, err
	}

	// cleanup packages that will be re-added from subprojects
	for imp := range tree.Packages {
		for _, gp := range p.Manifest.Meta.LocalGopaths {
			if strings.HasPrefix(imp, string(p.ImportRoot)+"/"+gp) {
				delete(tree.Packages, imp)
			}
		}
	}

	for _, sub := range p.SubProjects {
		t, _ := sub.ParseRootPackageTree()
		for imp, pack := range t.Packages {
			tree.Packages[imp] = pack
		}
	}
	return tree, nil
}

// HackExtraVendorEntries generates extra vendor entries for local packages
func (p *Project) HackExtraVendorEntries() error {
	vendorPath := filepath.Join(p.AbsRoot, "vendor")

	for imp, path := range p.extraVendorEntries {
		vendorProjectPath := filepath.Join(vendorPath, imp)
		vendorProjectDirPath := filepath.Dir(vendorProjectPath)
		os.MkdirAll(vendorProjectDirPath, 0755)
		relVendorProjectPath, _ := filepath.Rel(vendorProjectDirPath, path)
		_ = os.Symlink(relVendorProjectPath, vendorProjectPath)
	}
	return nil
}
