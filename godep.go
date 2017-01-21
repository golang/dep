package gps

import (
	"errors"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	ignoreTags = []string{} //[]string{"appengine", "ignore"} //TODO: appengine is a special case for now: https://github.com/tools/godep/issues/353
)

// fillPackage full of info. Assumes p.Dir is set at a minimum
func fillPackage(p *build.Package) error {
	if p.SrcRoot == "" {
		for _, base := range build.Default.SrcDirs() {
			if strings.HasPrefix(p.Dir, base) {
				p.SrcRoot = base
			}
		}
	}

	if p.SrcRoot == "" {
		return errors.New("Unable to find SrcRoot for package " + p.ImportPath)
	}

	if p.Root == "" {
		p.Root = filepath.Dir(p.SrcRoot)
	}

	var buildMatch = "+build "
	var buildFieldSplit = func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	}

	gofiles, err := filepath.Glob(filepath.Join(p.Dir, "*.go"))
	if err != nil {
		return err
	}

	if len(gofiles) == 0 {
		return &build.NoGoError{Dir: p.Dir}
	}

	var testImports []string
	var imports []string
NextFile:
	for _, file := range gofiles {
		pf, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		testFile := strings.HasSuffix(file, "_test.go")
		fname := filepath.Base(file)
		for _, c := range pf.Comments {
			if c.Pos() > pf.Package { // +build must come before package
				continue
			}
			ct := c.Text()
			if i := strings.Index(ct, buildMatch); i != -1 {
				for _, t := range strings.FieldsFunc(ct[i+len(buildMatch):], buildFieldSplit) {
					for _, tag := range ignoreTags {
						if t == tag {
							p.IgnoredGoFiles = append(p.IgnoredGoFiles, fname)
							continue NextFile
						}
					}
				}
			}
		}

		if testFile {
			p.TestGoFiles = append(p.TestGoFiles, fname)
			if p.Name == "" {
				p.Name = strings.TrimSuffix(pf.Name.Name, "_test")
			}
		} else {
			if p.Name == "" {
				p.Name = pf.Name.Name
			}
			p.GoFiles = append(p.GoFiles, fname)
		}

		for _, is := range pf.Imports {
			name, err := strconv.Unquote(is.Path.Value)
			if err != nil {
				return err // can't happen?
			}
			if testFile {
				testImports = append(testImports, name)
			} else {
				imports = append(imports, name)
			}
		}
	}

	imports = uniq(imports)
	testImports = uniq(testImports)
	p.Imports = imports
	p.TestImports = testImports
	return nil
}

func uniq(a []string) []string {
	if a == nil {
		return make([]string, 0)
	}
	var s string
	var i int
	if !sort.StringsAreSorted(a) {
		sort.Strings(a)
	}
	for _, t := range a {
		if t != s {
			a[i] = t
			i++
			s = t
		}
	}
	return a[:i]
}
