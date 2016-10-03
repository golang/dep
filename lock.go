package gps

import "sort"

// Lock represents data from a lock file (or however the implementing tool
// chooses to store it) at a particular version that is relevant to the
// satisfiability solving process.
//
// In general, the information produced by gps on finding a successful
// solution is all that would be necessary to constitute a lock file, though
// tools can include whatever other information they want in their storage.
type Lock interface {
	// Indicates the version of the solver used to generate this lock data
	//SolverVersion() string

	// The hash of inputs to gps that resulted in this lock data
	InputHash() []byte

	// Projects returns the list of LockedProjects contained in the lock data.
	Projects() []LockedProject
}

// LockedProject is a single project entry from a lock file. It expresses the
// project's name, one or both of version and underlying revision, the network
// URI for accessing it, the path at which it should be placed within a vendor
// directory, and the packages that are used in it.
type LockedProject struct {
	pi   ProjectIdentifier
	v    UnpairedVersion
	r    Revision
	pkgs []string
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

// NewLockedProject creates a new LockedProject struct with a given
// ProjectIdentifier (name and optional upstream source URL), version. and list
// of packages required from the project.
//
// Note that passing a nil version will cause a panic. This is a correctness
// measure to ensure that the solver is never exposed to a version-less lock
// entry. Such a case would be meaningless - the solver would have no choice but
// to simply dismiss that project. By creating a hard failure case via panic
// instead, we are trying to avoid inflicting the resulting pain on the user by
// instead forcing a decision on the Analyzer implementation.
func NewLockedProject(id ProjectIdentifier, v Version, pkgs []string) LockedProject {
	if v == nil {
		panic("must provide a non-nil version to create a LockedProject")
	}

	lp := LockedProject{
		pi:   id,
		pkgs: pkgs,
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

// Packages returns the list of packages from within the LockedProject that are
// actually used in the import graph. Some caveats:
//
//  * The names given are relative to the root import path for the project. If
//    the root package itself is imported, it's represented as ".".
//  * Just because a package path isn't included in this list doesn't mean it's
//    safe to remove - it could contain C files, or other assets, that can't be
//    safely removed.
//  * The slice is not a copy. If you need to modify it, copy it first.
func (lp LockedProject) Packages() []string {
	return lp.pkgs
}

func (lp LockedProject) toAtom() atomWithPackages {
	pa := atom{
		id: lp.Ident(),
	}

	if lp.v == nil {
		pa.v = lp.r
	} else if lp.r != "" {
		pa.v = lp.v.Is(lp.r)
	} else {
		pa.v = lp.v
	}

	return atomWithPackages{
		a:  pa,
		pl: lp.pkgs,
	}
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

// prepLock ensures a lock is prepared and safe for use by the solver. This is
// mostly about defensively ensuring that no outside routine can modify the lock
// while the solver is in-flight.
//
// This is achieved by copying the lock's data into a new safeLock.
func prepLock(l Lock) Lock {
	pl := l.Projects()

	rl := safeLock{
		h: l.InputHash(),
		p: make([]LockedProject, len(pl)),
	}
	copy(rl.p, pl)

	return rl
}

// SortLockedProjects sorts a slice of LockedProject in alphabetical order by
// ProjectRoot.
func SortLockedProjects(lps []LockedProject) {
	sort.Stable(lpsorter(lps))
}

type lpsorter []LockedProject

func (lps lpsorter) Swap(i, j int) {
	lps[i], lps[j] = lps[j], lps[i]
}

func (lps lpsorter) Len() int {
	return len(lps)
}

func (lps lpsorter) Less(i, j int) bool {
	return lps[i].pi.ProjectRoot < lps[j].pi.ProjectRoot
}
