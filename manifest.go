package vsolver

// Manifest represents the data from a manifest file (or however the
// implementing tool chooses to store it) at a particular version that is
// relevant to the satisfiability solving process:
//
// - A list of dependencies: project name, and a constraint
// - A list of development-time dependencies (e.g. for testing - only
// the root project's are incorporated)
//
// Finding a solution that satisfies the constraints expressed by all of these
// dependencies (and those from all other projects, transitively), is what the
// solver does.
//
// Note that vsolver does perform static analysis on all projects' codebases;
// if dependencies it finds through that analysis are missing from what the
// Manifest lists, it is considered an error that will eliminate that version
// from consideration in the solving algorithm.
type Manifest interface {
	Name() ProjectName
	GetDependencies() []ProjectDep
	GetDevDependencies() []ProjectDep
}

// SimpleManifest is a helper for tools to enumerate manifest data. It's
// generally intended for ephemeral manifests, such as those Analyzers create on
// the fly for projects with no manifest metadata, or metadata through a foreign
// tool's idioms.
type SimpleManifest struct {
	N  ProjectName
	P  []ProjectDep
	DP []ProjectDep
}

var _ Manifest = SimpleManifest{}

// Name returns the name of the project described by the manifest.
func (m SimpleManifest) Name() ProjectName {
	return m.N
}

// GetDependencies returns the project's dependencies.
func (m SimpleManifest) GetDependencies() []ProjectDep {
	return m.P
}

// GetDependencies returns the project's test dependencies.
func (m SimpleManifest) GetDevDependencies() []ProjectDep {
	return m.DP
}
