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

package dependencies

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// DepsBuilder provides a way to compute direct dependencies of a package
type DepsBuilder struct {
	Root          string
	Package       string
	LocalPackages []string
	SkipSubdirs   []string
	internal      *internalBuilder
}

type internalBuilder struct {
	Root          string
	Package       string
	LocalPackages map[string]interface{}
	SkipSubdirs   map[string]interface{}
}

func (b *DepsBuilder) compile() *internalBuilder {
	if b.internal != nil {
		return b.internal
	}

	loc := make(map[string]interface{})
	for _, p := range b.LocalPackages {
		loc[p] = nil
	}

	skip := make(map[string]interface{})
	for _, d := range b.SkipSubdirs {
		skip[d] = nil
	}

	return &internalBuilder{
		Root:          b.Root,
		Package:       b.Package,
		LocalPackages: loc,
		SkipSubdirs:   skip,
	}
}

// GetPackageDependencies gives back the list of direct dependencies for the current context
func (b *DepsBuilder) GetPackageDependencies() ([]string, error) {
	return b.compile().getPackageDependencies()
}

func (b *internalBuilder) getPackageDependencies() ([]string, error) {
	deps := make(map[string]interface{})

	err := filepath.Walk(b.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info == nil || !info.IsDir() {
			return nil
		}

		base := filepath.Base(path)
		if base == "vendor" || base == "testdata" || strings.HasPrefix(".", base) || strings.HasPrefix("_", base) {
			return filepath.SkipDir
		}

		if _, ok := b.SkipSubdirs[path]; ok {
			return filepath.SkipDir
		}

		subdeps, err := b.packageDeps(path)
		if err != nil {
			return err
		}

		for k := range subdeps {
			deps[k] = nil
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	res := make([]string, 0)
	for k := range deps {
		res = append(res, k)
	}
	return res, nil
}

func (b *internalBuilder) packageDeps(pack string) (map[string]interface{}, error) {
	depsMap := make(map[string]interface{})

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pack, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, i := range f.Imports {
				d := i.Path.Value
				d = d[1 : len(d)-1] // remove quotes
				if b.isExternalDependency(d) {
					depsMap[d] = nil
				}
			}
		}
	}

	return depsMap, nil
}

func (b *internalBuilder) isExternalDependency(pack string) bool {
	if strings.HasPrefix(pack, b.Package) {
		return false
	}

	for lp := range b.LocalPackages {
		if strings.HasPrefix(pack, lp) {
			return false
		}
	}

	cpts := strings.Split(pack, "/")
	lead := cpts[0]
	if strings.Contains(lead, ".") {
		return true
	}

	return false
}
