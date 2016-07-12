package vsolver

import (
	"fmt"
	"math/rand"
	"strconv"
)

// ProjectRoot is the topmost import path in a tree of other import paths - the
// root of the tree. In gps' current design, ProjectRoots have to correspond to
// a repository root (mostly), but their real purpose is to identify the root
// import path of a "project", logically encompassing all child packages.
//
// Projects are a crucial unit of operation in gps. Constraints are declared by
// a project's manifest, and apply to all packages in a ProjectRoot's tree.
// Solving itself mostly proceeds on a project-by-project basis.
//
// Aliasing string types is usually a bit of an anti-pattern. We do it here as a
// means of clarifying API intent. This is important because Go's package
// management domain has lots of different path-ish strings floating around:
//
//  actual directories:
//	/home/sdboyer/go/src/github.com/sdboyer/gps/example
//  URLs:
//	https://github.com/sdboyer/gps
//  import paths:
//	github.com/sdboyer/gps/example
//  portions of import paths that refer to a package:
//	example
//  portions that could not possibly refer to anything sane:
//	github.com/sdboyer
//  portions that correspond to a repository root:
//	github.com/sdboyer/gps
//
// While not a panacea, defining ProjectRoot at least allows us to clearly
// identify when one of these path-ish strings is *supposed* to have certain
// semantics.
type ProjectRoot string

// A ProjectIdentifier is, more or less, the name of a dependency. It is related
// to, but differs in two keys ways from, an import path.
//
// First, ProjectIdentifiers do not identify a single package. Rather, they
// encompasses the whole tree of packages that exist at or below their
// ProjectRoot. In gps' current design, this ProjectRoot must correspond to the
// root of a repository, though this may not always be the case.
//
// Second, ProjectIdentifiers can optionally carry a NetworkName, which
// identifies where the underlying source code can be located on the network.
// These can be either a full URL, including protocol, or plain import paths.
// So, these are all valid data for NetworkName:
//
//  github.com/sdboyer/gps
//  github.com/fork/gps
//  git@github.com:sdboyer/gps
//  https://github.com/sdboyer/gps
//
// With plain import paths, network addresses are derived purely through an
// algorithm. By having an explicit network name, it becomes possible to, for
// example, transparently substitute a fork for an original upstream repository.
//
// Note that gps makes no guarantees about the actual import paths contained in
// a repository aligning with ImportRoot. If tools, or their users, specify an
// alternate NetworkName that contains a repository with incompatible internal
// import paths, gps will fail. (gps does no import rewriting.)
//
// Also note that if different projects' manifests report a different
// NetworkName for a given ImportRoot, it is a solve failure. Everyone has to
// agree on where a given import path should be sourced from.
//
// If NetworkName is not explicitly set, gps will derive the network address from
// the ImportRoot using a similar algorithm to that of the official go tooling.
type ProjectIdentifier struct {
	ProjectRoot ProjectRoot
	NetworkName string
}

// A ProjectConstraint combines a ProjectIdentifier with a Constraint. It
// indicates that, if packages contained in the ProjectIdentifier enter the
// depgraph, they must do so at a version that is allowed by the Constraint.
type ProjectConstraint struct {
	Ident      ProjectIdentifier
	Constraint Constraint
}

func (i ProjectIdentifier) less(j ProjectIdentifier) bool {
	if i.ProjectRoot < j.ProjectRoot {
		return true
	}
	if j.ProjectRoot < i.ProjectRoot {
		return false
	}

	return i.NetworkName < j.NetworkName
}

func (i ProjectIdentifier) eq(j ProjectIdentifier) bool {
	if i.ProjectRoot != j.ProjectRoot {
		return false
	}
	if i.NetworkName == j.NetworkName {
		return true
	}

	if (i.NetworkName == "" && j.NetworkName == string(j.ProjectRoot)) ||
		(j.NetworkName == "" && i.NetworkName == string(i.ProjectRoot)) {
		return true
	}

	// TODO(sdboyer) attempt conversion to URL and compare base + path

	return false
}

func (i ProjectIdentifier) netName() string {
	if i.NetworkName == "" {
		return string(i.ProjectRoot)
	}
	return i.NetworkName
}

func (i ProjectIdentifier) errString() string {
	if i.NetworkName == "" || i.NetworkName == string(i.ProjectRoot) {
		return string(i.ProjectRoot)
	}
	return fmt.Sprintf("%s (from %s)", i.ProjectRoot, i.NetworkName)
}

func (i ProjectIdentifier) normalize() ProjectIdentifier {
	if i.NetworkName == "" {
		i.NetworkName = string(i.ProjectRoot)
	}

	return i
}

// Package represents a Go package. It contains a subset of the information
// go/build.Package does.
type Package struct {
	ImportPath, CommentPath string
	Name                    string
	Imports                 []string
	TestImports             []string
}

// bimodalIdentifiers are used to track work to be done in the unselected queue.
// TODO(sdboyer) marker for root, to know to ignore prefv...or can we do unselected queue
// sorting only?
type bimodalIdentifier struct {
	id ProjectIdentifier
	// List of packages required within/under the ProjectIdentifier
	pl []string
	// prefv is used to indicate a 'preferred' version. This is expected to be
	// derived from a dep's lock data, or else is empty.
	prefv Version
	// Indicates that the bmi came from the root project originally
	fromRoot bool
}

type atom struct {
	id ProjectIdentifier
	v  Version
}

// With a random revision and no name, collisions are...unlikely
var nilpa = atom{
	v: Revision(strconv.FormatInt(rand.Int63(), 36)),
}

type atomWithPackages struct {
	a  atom
	pl []string
}

//type byImportPath []Package

//func (s byImportPath) Len() int           { return len(s) }
//func (s byImportPath) Less(i, j int) bool { return s[i].ImportPath < s[j].ImportPath }
//func (s byImportPath) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// completeDep (name hopefully to change) provides the whole picture of a
// dependency - the root (repo and project, since currently we assume the two
// are the same) name, a constraint, and the actual packages needed that are
// under that root.
type completeDep struct {
	// The base ProjectDep
	ProjectConstraint
	// The specific packages required from the ProjectDep
	pl []string
}

type dependency struct {
	depender atom
	dep      completeDep
}
