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
	DependencyConstraints() ProjectConstraints

	// Returns a list of constraints applicable to test imports.
	//
	// These are applied only when tests are incorporated. Typically, that
	// will only be for root manifests.
	TestDependencyConstraints() ProjectConstraints
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
	Deps, TestDeps ProjectConstraints
}

var _ Manifest = SimpleManifest{}

// DependencyConstraints returns the project's dependencies.
func (m SimpleManifest) DependencyConstraints() ProjectConstraints {
	return m.Deps
}

// TestDependencyConstraints returns the project's test dependencies.
func (m SimpleManifest) TestDependencyConstraints() ProjectConstraints {
	return m.TestDeps
}

// simpleRootManifest exists so that we have a safe value to swap into solver
// params when a nil Manifest is provided.
//
// Also, for tests.
type simpleRootManifest struct {
	c, tc, ovr ProjectConstraints
	ig         map[string]bool
}

func (m simpleRootManifest) DependencyConstraints() ProjectConstraints {
	return m.c
}
func (m simpleRootManifest) TestDependencyConstraints() ProjectConstraints {
	return m.tc
}
func (m simpleRootManifest) Overrides() ProjectConstraints {
	return m.ovr
}
func (m simpleRootManifest) IgnorePackages() map[string]bool {
	return m.ig
}
func (m simpleRootManifest) dup() simpleRootManifest {
	m2 := simpleRootManifest{
		c:   make(ProjectConstraints, len(m.c)),
		tc:  make(ProjectConstraints, len(m.tc)),
		ovr: make(ProjectConstraints, len(m.ovr)),
		ig:  make(map[string]bool, len(m.ig)),
	}

	for k, v := range m.c {
		m2.c[k] = v
	}
	for k, v := range m.tc {
		m2.tc[k] = v
	}
	for k, v := range m.ovr {
		m2.ovr[k] = v
	}
	for k, v := range m.ig {
		m2.ig[k] = v
	}

	return m2
}

// prepManifest ensures a manifest is prepared and safe for use by the solver.
// This is mostly about ensuring that no outside routine can modify the manifest
// while the solver is in-flight.
//
// This is achieved by copying the manifest's data into a new SimpleManifest.
func prepManifest(m Manifest) Manifest {
	if m == nil {
		return SimpleManifest{}
	}

	deps := m.DependencyConstraints()
	ddeps := m.TestDependencyConstraints()

	rm := SimpleManifest{
		Deps:     make(ProjectConstraints, len(deps)),
		TestDeps: make(ProjectConstraints, len(ddeps)),
	}

	for k, d := range deps {
		rm.Deps[k] = d
	}
	for k, d := range ddeps {
		rm.TestDeps[k] = d
	}

	return rm
}
