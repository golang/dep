package gps

import (
	"fmt"
	"go/build"
	"go/parser"
	gscan "go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	osList     []string
	archList   []string
	ignoreTags = []string{} //[]string{"appengine", "ignore"} //TODO: appengine is a special case for now: https://github.com/tools/godep/issues/353
)

func init() {
	// The supported systems are listed in
	// https://github.com/golang/go/blob/master/src/go/build/syslist.go
	// The lists are not exported, so we need to duplicate them here.
	osListString := "android darwin dragonfly freebsd linux nacl netbsd openbsd plan9 solaris windows"
	osList = strings.Split(osListString, " ")

	archListString := "386 amd64 amd64p32 arm armbe arm64 arm64be ppc64 ppc64le mips mipsle mips64 mips64le mips64p32 mips64p32le ppc s390 s390x sparc sparc64"
	archList = strings.Split(archListString, " ")
}

// Stored as a var so that tests can swap it out. Ugh globals, ugh.
var isStdLib = doIsStdLib

// This was loving taken from src/cmd/go/pkg.go in Go's code (isStandardImportPath).
func doIsStdLib(path string) bool {
	i := strings.Index(path, "/")
	if i < 0 {
		i = len(path)
	}

	return !strings.Contains(path[:i], ".")
}

// ListPackages reports Go package information about all directories in the tree
// at or below the provided fileRoot.
//
// Directories without any valid Go files are excluded. Directories with
// multiple packages are excluded.
//
// The importRoot parameter is prepended to the relative path when determining
// the import path for each package. The obvious case is for something typical,
// like:
//
//  fileRoot = "/home/user/go/src/github.com/foo/bar"
//  importRoot = "github.com/foo/bar"
//
// where the fileRoot and importRoot align. However, if you provide:
//
//  fileRoot = "/home/user/workspace/path/to/repo"
//  importRoot = "github.com/foo/bar"
//
// then the root package at path/to/repo will be ascribed import path
// "github.com/foo/bar", and the package at
// "/home/user/workspace/path/to/repo/baz" will be "github.com/foo/bar/baz".
//
// A PackageTree is returned, which contains the ImportRoot and map of import path
// to PackageOrErr - each path under the root that exists will have either a
// Package, or an error describing why the directory is not a valid package.
func ListPackages(fileRoot, importRoot string) (PackageTree, error) {
	ptree := PackageTree{
		ImportRoot: importRoot,
		Packages:   make(map[string]PackageOrErr),
	}

	var err error
	fileRoot, err = filepath.Abs(fileRoot)
	if err != nil {
		return PackageTree{}, err
	}

	err = filepath.Walk(fileRoot, func(wp string, fi os.FileInfo, err error) error {
		if err != nil && err != filepath.SkipDir {
			return err
		}

		// Read the destination of named symbolic link
		if fi, err := readSymlink(wp, fileRoot, fi); err != nil {
			return nil
		} else if !fi.IsDir() {
			return nil
		}

		// Skip dirs that are known to hold non-local/dependency code.
		//
		// We don't skip _*, or testdata dirs because, while it may be poor
		// form, importing them is not a compilation error.
		switch fi.Name() {
		case "vendor", "Godeps":
			return filepath.SkipDir
		}
		// We do skip dot-dirs, though, because it's such a ubiquitous standard
		// that they not be visited by normal commands, and because things get
		// really weird if we don't.
		if strings.HasPrefix(fi.Name(), ".") {
			return filepath.SkipDir
		}

		// Compute the import path. Run the result through ToSlash(), so that windows
		// paths are normalized to Unix separators, as import paths are expected
		// to be.
		ip := filepath.ToSlash(filepath.Join(importRoot, strings.TrimPrefix(wp, fileRoot)))

		// Find all the imports, across all os/arch combos
		//p, err := fullPackageInDir(wp)
		p := &build.Package{
			Dir: wp,
		}
		err = fillPackage(p)

		var pkg Package
		if err == nil {
			pkg = Package{
				ImportPath:  ip,
				CommentPath: p.ImportComment,
				Name:        p.Name,
				Imports:     p.Imports,
				TestImports: dedupeStrings(p.TestImports, p.XTestImports),
			}
		} else {
			switch err.(type) {
			case gscan.ErrorList, *gscan.Error, *build.NoGoError:
				// This happens if we encounter malformed or nonexistent Go
				// source code
				ptree.Packages[ip] = PackageOrErr{
					Err: err,
				}
				return nil
			default:
				return err
			}
		}

		// This area has some...fuzzy rules, but check all the imports for
		// local/relative/dot-ness, and record an error for the package if we
		// see any.
		var lim []string
		for _, imp := range append(pkg.Imports, pkg.TestImports...) {
			switch {
			// Do allow the single-dot, at least for now
			case imp == "..":
				lim = append(lim, imp)
				// ignore stdlib done this way, b/c that's what the go tooling does
			case strings.HasPrefix(imp, "./"):
				if isStdLib(imp[2:]) {
					lim = append(lim, imp)
				}
			case strings.HasPrefix(imp, "../"):
				if isStdLib(imp[3:]) {
					lim = append(lim, imp)
				}
			}
		}

		if len(lim) > 0 {
			ptree.Packages[ip] = PackageOrErr{
				Err: &LocalImportsError{
					Dir:          ip,
					LocalImports: lim,
				},
			}
		} else {
			ptree.Packages[ip] = PackageOrErr{
				P: pkg,
			}
		}

		return nil
	})

	if err != nil {
		return PackageTree{}, err
	}

	return ptree, nil
}

func readSymlink(wp, fileRoot string, fi os.FileInfo) (os.FileInfo, error) {
	// read only symlink dir
	if fi.IsDir() || fi.Mode()&os.ModeSymlink == 0 {
		return fi, nil
	}

	dst, err := os.Readlink(wp)
	if err != nil {
		return fi, err
	}

	// All absolute symlinks are disqualified; if one is encountered, it should be skipped.
	if filepath.IsAbs(dst) {
		return fi, nil
	}

	// Relative symlinks pointing to somewhere outside of the root (via ..) should also be skipped.
	dst, err = filepath.EvalSymlinks(wp)
	if err != nil {
		return fi, nil
	} else if !strings.HasPrefix(dst, fileRoot) {
		return fi, nil
	}
	return os.Lstat(dst)
}

// fillPackage full of info. Assumes p.Dir is set at a minimum
func fillPackage(p *build.Package) error {
	var buildPrefix = "// +build "
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
	for _, file := range gofiles {
		pf, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		testFile := strings.HasSuffix(file, "_test.go")
		fname := filepath.Base(file)

		var ignored bool
		for _, c := range pf.Comments {
			if c.Pos() > pf.Package { // +build comment must come before package
				continue
			}

			var ct string
			for _, cl := range c.List {
				if strings.HasPrefix(cl.Text, buildPrefix) {
					ct = cl.Text
					break
				}
			}
			if ct == "" {
				continue
			}

			for _, t := range strings.FieldsFunc(ct[len(buildPrefix):], buildFieldSplit) {
				// hardcoded (for now) handling for the "ignore" build tag
				// We "soft" ignore the files tagged with ignore so that we pull in their imports.
				if t == "ignore" {
					ignored = true
				}
			}
		}

		if testFile {
			p.TestGoFiles = append(p.TestGoFiles, fname)
			if p.Name == "" && !ignored {
				p.Name = strings.TrimSuffix(pf.Name.Name, "_test")
			}
		} else {
			if p.Name == "" && !ignored {
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

// LocalImportsError indicates that a package contains at least one relative
// import that will prevent it from compiling.
//
// TODO(sdboyer) add a Files property once we're doing our own per-file parsing
type LocalImportsError struct {
	Dir          string
	LocalImports []string
}

func (e *LocalImportsError) Error() string {
	return fmt.Sprintf("import path %s had problematic local imports", e.Dir)
}

// A PackageTree represents the results of recursively parsing a tree of
// packages, starting at the ImportRoot. The results of parsing the files in the
// directory identified by each import path - a Package or an error - are stored
// in the Packages map, keyed by that import path.
type PackageTree struct {
	ImportRoot string
	Packages   map[string]PackageOrErr
}

// dup copies the PackageTree.
//
// This is really only useful as a defensive measure to prevent external state
// mutations.
func (t PackageTree) dup() PackageTree {
	t2 := PackageTree{
		ImportRoot: t.ImportRoot,
		Packages:   map[string]PackageOrErr{},
	}

	for path, poe := range t.Packages {
		poe2 := PackageOrErr{
			Err: poe.Err,
			P:   poe.P,
		}
		if len(poe.P.Imports) > 0 {
			poe2.P.Imports = make([]string, len(poe.P.Imports))
			copy(poe2.P.Imports, poe.P.Imports)
		}
		if len(poe.P.TestImports) > 0 {
			poe2.P.TestImports = make([]string, len(poe.P.TestImports))
			copy(poe2.P.TestImports, poe.P.TestImports)
		}

		t2.Packages[path] = poe2
	}

	return t2
}

type wm struct {
	err error
	ex  map[string]bool
	in  map[string]bool
}

// PackageOrErr stores the results of attempting to parse a single directory for
// Go source code.
type PackageOrErr struct {
	P   Package
	Err error
}

// ReachMap maps a set of import paths (keys) to the set of external packages
// transitively reachable from the packages at those import paths.
//
// See PackageTree.ExternalReach() for more information.
type ReachMap map[string][]string

// ExternalReach looks through a PackageTree and computes the list of external
// import statements (that is, import statements pointing to packages that are
// not logical children of PackageTree.ImportRoot) that are transitively
// imported by the internal packages in the tree.
//
// main indicates whether (true) or not (false) to include main packages in the
// analysis. When utilized by gps' solver, main packages are generally excluded
// from analyzing anything other than the root project, as they necessarily can't
// be imported.
//
// tests indicates whether (true) or not (false) to include imports from test
// files in packages when computing the reach map.
//
// ignore is a map of import paths that, if encountered, should be excluded from
// analysis. This exclusion applies to both internal and external packages. If
// an external import path is ignored, it is simply omitted from the results.
//
// If an internal path is ignored, then not only does it not appear in the final
// map, but it is also excluded from the transitive calculations of other
// internal packages.  That is, if you ignore A/foo, then the external package
// list for all internal packages that import A/foo will not include external
// packages that are only reachable through A/foo.
//
// Visually, this means that, given a PackageTree with root A and packages at A,
// A/foo, and A/bar, and the following import chain:
//
//  A -> A/foo -> A/bar -> B/baz
//
// In this configuration, all of A's packages transitively import B/baz, so the
// returned map would be:
//
//  map[string][]string{
// 	"A": []string{"B/baz"},
// 	"A/foo": []string{"B/baz"}
// 	"A/bar": []string{"B/baz"},
//  }
//
// However, if you ignore A/foo, then A's path to B/baz is broken, and A/foo is
// omitted entirely. Thus, the returned map would be:
//
//  map[string][]string{
// 	"A": []string{},
// 	"A/bar": []string{"B/baz"},
//  }
//
// If there are no packages to ignore, it is safe to pass a nil map.
func (t PackageTree) ExternalReach(main, tests bool, ignore map[string]bool) ReachMap {
	if ignore == nil {
		ignore = make(map[string]bool)
	}

	// world's simplest adjacency list
	workmap := make(map[string]wm)

	var imps []string
	for ip, perr := range t.Packages {
		if perr.Err != nil {
			workmap[ip] = wm{
				err: perr.Err,
			}
			continue
		}
		p := perr.P

		// Skip main packages, unless param says otherwise
		if p.Name == "main" && !main {
			continue
		}
		// Skip ignored packages
		if ignore[ip] {
			continue
		}

		imps = imps[:0]
		imps = p.Imports
		if tests {
			imps = dedupeStrings(imps, p.TestImports)
		}

		w := wm{
			ex: make(map[string]bool),
			in: make(map[string]bool),
		}

		for _, imp := range imps {
			// Skip ignored imports
			if ignore[imp] {
				continue
			}

			if !eqOrSlashedPrefix(imp, t.ImportRoot) {
				w.ex[imp] = true
			} else {
				if w2, seen := workmap[imp]; seen {
					for i := range w2.ex {
						w.ex[i] = true
					}
					for i := range w2.in {
						w.in[i] = true
					}
				} else {
					w.in[imp] = true
				}
			}
		}

		workmap[ip] = w
	}

	//return wmToReach(workmap, t.ImportRoot)
	return wmToReach(workmap, "") // TODO(sdboyer) this passes tests, but doesn't seem right
}

// wmToReach takes an internal "workmap" constructed by
// PackageTree.ExternalReach(), transitively walks (via depth-first traversal)
// all internal imports until they reach an external path or terminate, then
// translates the results into a slice of external imports for each internal
// pkg.
//
// The basedir string, with a trailing slash ensured, will be stripped from the
// keys of the returned map.
//
// This is mostly separated out for testing purposes.
func wmToReach(workmap map[string]wm, basedir string) map[string][]string {
	// Uses depth-first exploration to compute reachability into external
	// packages, dropping any internal packages on "poisoned paths" - a path
	// containing a package with an error, or with a dep on an internal package
	// that's missing.

	const (
		white uint8 = iota
		grey
		black
	)

	colors := make(map[string]uint8)
	allreachsets := make(map[string]map[string]struct{})

	// poison is a helper func to eliminate specific reachsets from allreachsets
	poison := func(path []string) {
		for _, ppkg := range path {
			delete(allreachsets, ppkg)
		}
	}

	var dfe func(string, []string) bool

	// dfe is the depth-first-explorer that computes a safe, error-free external
	// reach map.
	//
	// pkg is the import path of the pkg currently being visited; path is the
	// stack of parent packages we've visited to get to pkg. The return value
	// indicates whether the level completed successfully (true) or if it was
	// poisoned (false).
	//
	// TODO(sdboyer) some deft improvements could probably be made by passing the list of
	// parent reachsets, rather than a list of parent package string names.
	// might be able to eliminate the use of allreachsets map-of-maps entirely.
	dfe = func(pkg string, path []string) bool {
		// white is the zero value of uint8, which is what we want if the pkg
		// isn't in the colors map, so this works fine
		switch colors[pkg] {
		case white:
			// first visit to this pkg; mark it as in-process (grey)
			colors[pkg] = grey

			// make sure it's present and w/out errs
			w, exists := workmap[pkg]
			if !exists || w.err != nil {
				// Does not exist or has an err; poison self and all parents
				poison(path)

				// we know we're done here, so mark it black
				colors[pkg] = black
				return false
			}
			// pkg exists with no errs. mark it as in-process (grey), and start
			// a reachmap for it
			//
			// TODO(sdboyer) use sync.Pool here? can be lots of explicit map alloc/dealloc
			rs := make(map[string]struct{})

			// Push self onto the path slice. Passing this as a value has the
			// effect of auto-popping the slice, while also giving us safe
			// memory reuse.
			path = append(path, pkg)

			// Dump this package's external pkgs into its own reachset. Separate
			// loop from the parent dump to avoid nested map loop lookups.
			for ex := range w.ex {
				rs[ex] = struct{}{}
			}
			allreachsets[pkg] = rs

			// Push this pkg's external imports into all parent reachsets. Not
			// all parents will necessarily have a reachset; none, some, or all
			// could have been poisoned by a different path than what we're on
			// right now. (Or we could be at depth 0)
			for _, ppkg := range path {
				if prs, exists := allreachsets[ppkg]; exists {
					for ex := range w.ex {
						prs[ex] = struct{}{}
					}
				}
			}

			// Now, recurse until done, or a false bubbles up, indicating the
			// path is poisoned.
			var clean bool
			for in := range w.in {
				// It's possible, albeit weird, for a package to import itself.
				// If we try to visit self, though, then it erroneously poisons
				// the path, as it would be interpreted as grey. In reality,
				// this becomes a no-op, so just skip it.
				if in == pkg {
					continue
				}

				clean = dfe(in, path)
				if !clean {
					// Path is poisoned. Our reachmap was already deleted by the
					// path we're returning from; mark ourselves black, then
					// bubble up the poison. This is OK to do early, before
					// exploring all internal imports, because the outer loop
					// visits all internal packages anyway.
					//
					// In fact, stopping early is preferable - white subpackages
					// won't have to iterate pointlessly through a parent path
					// with no reachset.
					colors[pkg] = black
					return false
				}
			}

			// Fully done with this pkg; no transitive problems.
			colors[pkg] = black
			return true

		case grey:
			// Import cycles can arise in healthy situations through xtests, so
			// allow them for now.
			//
			// FIXME(sdboyer) we need an improved model that allows us to
			// accurately reject real import cycles.
			return true
			// grey means an import cycle; guaranteed badness right here. You'd
			// hope we never encounter it in a dependency (really? you published
			// that code?), but we have to defend against it.
			//colors[pkg] = black
			//poison(append(path, pkg)) // poison self and parents

		case black:
			// black means we're done with the package. If it has an entry in
			// allreachsets, it completed successfully. If not, it was poisoned,
			// and we need to bubble the poison back up.
			rs, exists := allreachsets[pkg]
			if !exists {
				// just poison parents; self was necessarily already poisoned
				poison(path)
				return false
			}

			// It's good; pull over of the external imports from its reachset
			// into all non-poisoned parent reachsets
			for _, ppkg := range path {
				if prs, exists := allreachsets[ppkg]; exists {
					for ex := range rs {
						prs[ex] = struct{}{}
					}
				}
			}
			return true

		default:
			panic(fmt.Sprintf("invalid color marker %v for %s", colors[pkg], pkg))
		}
	}

	// Run the depth-first exploration.
	//
	// Don't bother computing graph sources, this straightforward loop works
	// comparably well, and fits nicely with an escape hatch in the dfe.
	var path []string
	for pkg := range workmap {
		dfe(pkg, path)
	}

	if len(allreachsets) == 0 {
		return nil
	}

	// Flatten allreachsets into the final reachlist
	rt := strings.TrimSuffix(basedir, string(os.PathSeparator)) + string(os.PathSeparator)
	rm := make(map[string][]string)
	for pkg, rs := range allreachsets {
		rlen := len(rs)
		if rlen == 0 {
			rm[strings.TrimPrefix(pkg, rt)] = nil
			continue
		}

		edeps := make([]string, rlen)
		k := 0
		for opkg := range rs {
			edeps[k] = opkg
			k++
		}

		sort.Strings(edeps)
		rm[strings.TrimPrefix(pkg, rt)] = edeps
	}

	return rm
}

// ListExternalImports computes a sorted, deduplicated list of all the external
// packages that are reachable through imports from all valid packages in a
// ReachMap, as computed by PackageTree.ExternalReach().
//
// main and tests determine whether main packages and test imports should be
// included in the calculation. "External" is defined as anything not prefixed,
// after path cleaning, by the PackageTree.ImportRoot. This includes stdlib.
//
// If an internal path is ignored, all of the external packages that it uniquely
// imports are omitted. Note, however, that no internal transitivity checks are
// made here - every non-ignored package in the tree is considered independently
// (with one set of exceptions, noted below). That means, given a PackageTree
// with root A and packages at A, A/foo, and A/bar, and the following import
// chain:
//
//  A -> A/foo -> A/bar -> B/baz
//
// If you ignore A or A/foo, A/bar will still be visited, and B/baz will be
// returned, because this method visits ALL packages in the tree, not only those reachable
// from the root (or any other) packages. If your use case requires interrogating
// external imports with respect to only specific package entry points, you need
// ExternalReach() instead.
//
// It is safe to pass a nil map if there are no packages to ignore.
//
// If an internal package has an error (that is, PackageOrErr is Err), it is excluded from
// consideration. Internal packages that transitively import the error package
// are also excluded. So, if:
//
//    -> B/foo
//   /
//  A
//   \
//    -> A/bar -> B/baz
//
// And A/bar has some error in it, then both A and A/bar will be eliminated from
// consideration; neither B/foo nor B/baz will be in the results. If A/bar, with
// its errors, is ignored, however, then A will remain, and B/foo will be in the
// results.
//
// Finally, note that if a directory is named "testdata", or has a leading dot
// or underscore, it will not be directly analyzed as a source. This is in
// keeping with Go tooling conventions that such directories should be ignored.
// So, if:
//
//  A -> B/foo
//  A/.bar -> B/baz
//  A/_qux -> B/baz
//  A/testdata -> B/baz
//
// Then B/foo will be returned, but B/baz will not, because all three of the
// packages that import it are in directories with disallowed names.
//
// HOWEVER, in keeping with the Go compiler, if one of those packages in a
// disallowed directory is imported by a package in an allowed directory, then
// it *will* be used. That is, while tools like go list will ignore a directory
// named .foo, you can still import from .foo. Thus, it must be included. So,
// if:
//
//    -> B/foo
//   /
//  A
//   \
//    -> A/.bar -> B/baz
//
// A is legal, and it imports A/.bar, so the results will include B/baz.
func (rm ReachMap) ListExternalImports() []string {
	exm := make(map[string]struct{})
	for pkg, reach := range rm {
		// Eliminate import paths with any elements having leading dots, leading
		// underscores, or testdata. If these are internally reachable (which is
		// a no-no, but possible), any external imports will have already been
		// pulled up through ExternalReach. The key here is that we don't want
		// to treat such packages as themselves being sources.
		//
		// TODO(sdboyer) strings.Split will always heap alloc, which isn't great to do
		// in a loop like this. We could also just parse it ourselves...
		var skip bool
		for _, elem := range strings.Split(pkg, "/") {
			if strings.HasPrefix(elem, ".") || strings.HasPrefix(elem, "_") || elem == "testdata" {
				skip = true
				break
			}
		}

		if !skip {
			for _, ex := range reach {
				exm[ex] = struct{}{}
			}
		}
	}

	if len(exm) == 0 {
		return nil
	}

	ex := make([]string, len(exm))
	k := 0
	for p := range exm {
		ex[k] = p
		k++
	}

	sort.Strings(ex)
	return ex
}

// eqOrSlashedPrefix checks to see if the prefix is either equal to the string,
// or that it is a prefix and the next char in the string is "/".
func eqOrSlashedPrefix(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}

	prflen, pathlen := len(prefix), len(s)
	return prflen == pathlen || strings.Index(s[prflen:], "/") == 0
}

// helper func to merge, dedupe, and sort strings
func dedupeStrings(s1, s2 []string) (r []string) {
	dedupe := make(map[string]bool)

	if len(s1) > 0 && len(s2) > 0 {
		for _, i := range s1 {
			dedupe[i] = true
		}
		for _, i := range s2 {
			dedupe[i] = true
		}

		for i := range dedupe {
			r = append(r, i)
		}
		// And then re-sort them
		sort.Strings(r)
	} else if len(s1) > 0 {
		r = s1
	} else if len(s2) > 0 {
		r = s2
	}

	return
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
