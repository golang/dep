package gps

// Manifest represents manifest-type data for a project at a particular version.
// That means dependency constraints, both for normal dependencies and for
// tests. The constraints expressed in a manifest determine the set of versions that
// are acceptable to try for a given project.
//
// Expressing a constraint in a manifest does not guarantee that a particular
// dependency will be present. It only guarantees that if packages in the
// project specified by the dependency are discovered through static analysis of
// the (transitive) import graph, then they will conform to the constraint.
//
// This does entail that manifests can express constraints on projects they do
// not themselves import. This is by design, but its implications are complex.
// See the gps docs for more information: https://github.com/sdboyer/gps/wiki
type Manifest interface {
	// Returns a list of project-level constraints.
	DependencyConstraints() []ProjectConstraint

	// Returns a list of constraints applicable to test imports.
	//
	// These are applied only when tests are incorporated. Typically, that
	// will only be for root manifests.
	TestDependencyConstraints() []ProjectConstraint
}

// RootManifest extends Manifest to add special controls over solving that are
// only afforded to the root project.
type RootManifest interface {
	Manifest

	// Overrides returns a list of ProjectConstraints that will unconditionally
	// supercede any ProjectConstraint declarations made in either the root
	// manifest, or in any dependency's manifest.
	//
	// Overrides are a special control afforded only to root manifests. Tool
	// users should be encouraged to use them only as a last resort; they do not
	// "play well with others" (that is their express goal), and overreliance on
	// them can harm the ecosystem as a whole.
	Overrides() ProjectConstraints

	// IngorePackages returns a set of import paths to ignore. These import
	// paths can be within the root project, or part of other projects. Ignoring
	// a package means that both it and its (unique) imports will be disregarded
	// by all relevant solver operations.
	IgnorePackages() map[string]bool
}

// SimpleManifest is a helper for tools to enumerate manifest data. It's
// generally intended for ephemeral manifests, such as those Analyzers create on
// the fly for projects with no manifest metadata, or metadata through a foreign
// tool's idioms.
type SimpleManifest struct {
	Deps     []ProjectConstraint
	TestDeps []ProjectConstraint
}

var _ Manifest = SimpleManifest{}

// DependencyConstraints returns the project's dependencies.
func (m SimpleManifest) DependencyConstraints() []ProjectConstraint {
	return m.Deps
}

// TestDependencyConstraints returns the project's test dependencies.
func (m SimpleManifest) TestDependencyConstraints() []ProjectConstraint {
	return m.TestDeps
}

// simpleRootManifest exists so that we have a safe value to swap into solver
// params when a nil Manifest is provided.
//
// Also, for tests.
type simpleRootManifest struct {
	c   []ProjectConstraint
	tc  []ProjectConstraint
	ovr ProjectConstraints
	ig  map[string]bool
}

func (m simpleRootManifest) DependencyConstraints() []ProjectConstraint {
	return m.c
}
func (m simpleRootManifest) TestDependencyConstraints() []ProjectConstraint {
	return m.tc
}
func (m simpleRootManifest) Overrides() ProjectConstraints {
	return m.ovr
}
func (m simpleRootManifest) IgnorePackages() map[string]bool {
	return m.ig
}

// prepManifest ensures a manifest is prepared and safe for use by the solver.
// This entails two things:
//
//  * Ensuring that all ProjectIdentifiers are normalized (otherwise matching
//  can get screwy and the queues go out of alignment)
//  * Defensively ensuring that no outside routine can modify the manifest while
//  the solver is in-flight.
//
// This is achieved by copying the manifest's data into a new SimpleManifest.
func prepManifest(m Manifest) Manifest {
	if m == nil {
		return SimpleManifest{}
	}

	deps := m.DependencyConstraints()
	ddeps := m.TestDependencyConstraints()

	rm := SimpleManifest{
		Deps:     make([]ProjectConstraint, len(deps)),
		TestDeps: make([]ProjectConstraint, len(ddeps)),
	}

	for k, d := range deps {
		d.Ident = d.Ident.normalize()
		rm.Deps[k] = d
	}
	for k, d := range ddeps {
		d.Ident = d.Ident.normalize()
		rm.TestDeps[k] = d
	}

	return rm
}
