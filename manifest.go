package vsolver

// Manifest represents the data from a manifest file (or however the
// implementing tool chooses to store it) at a particular version that is
// relevant to the satisfiability solving process. That means constraints on
// dependencies, both for normal dependencies and for tests.
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
	DependencyConstraints() []ProjectDep
	TestDependencyConstraints() []ProjectDep
}

// SimpleManifest is a helper for tools to enumerate manifest data. It's
// generally intended for ephemeral manifests, such as those Analyzers create on
// the fly for projects with no manifest metadata, or metadata through a foreign
// tool's idioms.
type SimpleManifest struct {
	N        ProjectName
	Deps     []ProjectDep
	TestDeps []ProjectDep
}

var _ Manifest = SimpleManifest{}

// Name returns the name of the project described by the manifest.
func (m SimpleManifest) Name() ProjectName {
	return m.N
}

// GetDependencies returns the project's dependencies.
func (m SimpleManifest) DependencyConstraints() []ProjectDep {
	return m.Deps
}

// GetDependencies returns the project's test dependencies.
func (m SimpleManifest) TestDependencyConstraints() []ProjectDep {
	return m.TestDeps
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
func prepManifest(m Manifest, n ProjectName) Manifest {
	if m == nil {
		// Only use the provided ProjectName if making an empty manifest;
		// otherwise, we trust the input manifest.
		return SimpleManifest{
			N: n,
		}
	}

	deps := m.DependencyConstraints()
	ddeps := m.TestDependencyConstraints()

	rm := SimpleManifest{
		N:        m.Name(),
		Deps:     make([]ProjectDep, len(deps)),
		TestDeps: make([]ProjectDep, len(ddeps)),
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
