package vsolver

// Lock represents data from a lock file (or however the implementing tool
// chooses to store it) at a particular version that is relevant to the
// satisfiability solving process.
//
// In general, the information produced by vsolver on finding a successful
// solution is all that would be necessary to constitute a lock file, though
// tools can include whatever other information they want in their storage.
type Lock interface {
	// Indicates the version of the solver used to generate this lock data
	//SolverVersion() string

	// The hash of inputs to vsolver that resulted in this lock data
	InputHash() []byte

	// Projects returns the list of LockedProjects contained in the lock data.
	Projects() []LockedProject
}

// LockedProject is a single project entry from a lock file. It expresses the
// project's name, one or both of version and underlying revision, the network
// URI for accessing it, and the path at which it should be placed within a
// vendor directory.
//
// TODO note that sometime soon, we also plan to allow pkgs. this'll change
type LockedProject struct {
	pi   ProjectIdentifier
	v    UnpairedVersion
	r    Revision
	path string
}

// SimpleLock is a helper for tools to easily describe lock data when they know
// that no hash, or other complex information, is available.
type SimpleLock []LockedProject

var _ Lock = SimpleLock{}

// InputHash always returns an empty string for SimpleLock. This makes it useless
// as a stable lock to be written to disk, but still useful for some ephemeral
// purposes.
func (SimpleLock) InputHash() []byte {
	return nil
}

// Projects returns the entire contents of the SimpleLock.
func (l SimpleLock) Projects() []LockedProject {
	return l
}

// NewLockedProject creates a new LockedProject struct with a given name,
// version, upstream repository URI, and on-disk path at which the project is to
// be checked out under a vendor directory.
//
// Note that passing a nil version will cause a panic. This is a correctness
// measure to ensure that the solver is never exposed to a version-less lock
// entry. Such a case would be meaningless - the solver would have no choice but
// to simply dismiss that project. By creating a hard failure case via panic
// instead, we are trying to avoid inflicting the resulting pain on the user by
// instead forcing a decision on the Analyzer implementation.
func NewLockedProject(n ProjectName, v Version, uri, path string) LockedProject {
	if v == nil {
		panic("must provide a non-nil version to create a LockedProject")
	}

	lp := LockedProject{
		pi: ProjectIdentifier{
			LocalName:   n,
			NetworkName: uri,
		},
		path: path,
	}

	switch tv := v.(type) {
	case Revision:
		lp.r = tv
	case branchVersion:
		lp.v = tv
	case semVersion:
		lp.v = tv
	case plainVersion:
		lp.v = tv
	case versionPair:
		lp.r = tv.r
		lp.v = tv.v
	}

	return lp
}

// Ident returns the identifier describing the project. This includes both the
// local name (the root name by which the project is referenced in import paths)
// and the network name, where the upstream source lives.
func (lp LockedProject) Ident() ProjectIdentifier {
	return lp.pi
}

// Version assembles together whatever version and/or revision data is
// available into a single Version.
func (lp LockedProject) Version() Version {
	if lp.r == "" {
		return lp.v
	}

	if lp.v == nil {
		return lp.r
	}

	return lp.v.Is(lp.r)
}

// Path returns the path relative to the vendor directory to which the locked
// project should be checked out.
func (lp LockedProject) Path() string {
	return lp.path
}

func (lp LockedProject) toAtom() ProjectAtom {
	pa := ProjectAtom{
		Ident: lp.Ident(),
	}

	if lp.v == nil {
		pa.Version = lp.r
	} else if lp.r != "" {
		pa.Version = lp.v.Is(lp.r)
	} else {
		pa.Version = lp.v
	}

	return pa
}

type safeLock struct {
	h []byte
	p []LockedProject
}

func (sl safeLock) InputHash() []byte {
	return sl.h
}

func (sl safeLock) Projects() []LockedProject {
	return sl.p
}

// prepLock ensures a lock is prepared and safe for use by the solver.
// This entails two things:
//
//  * Ensuring that all LockedProject's identifiers are normalized.
//  * Defensively ensuring that no outside routine can modify the lock while the
//  solver is in-flight.
//
// This is achieved by copying the lock's data into a new safeLock.
func prepLock(l Lock) Lock {
	pl := l.Projects()

	rl := safeLock{
		h: l.InputHash(),
		p: make([]LockedProject, len(pl)),
	}

	for k, lp := range pl {
		lp.pi = lp.pi.normalize()
		rl.p[k] = lp
	}

	return rl
}
