package vsolver

import "sort"

// smAdapter is an adapter and around a proper SourceManager.
//
// It provides localized caching that's tailored to the requirements of a
// particular solve run.
//
// It also performs transformations between ProjectIdentifiers, which is what
// the solver primarily deals in, and ProjectName, which is what the
// SourceManager primarily deals in. This separation is helpful because it keeps
// the complexities of deciding what a particular name "means" entirely within
// the solver, while the SourceManager can traffic exclusively in
// globally-unique network names.
//
// Finally, it provides authoritative version/constraint operations, ensuring
// that any possible approach to a match - even those not literally encoded in
// the inputs - is achieved.
type smAdapter struct {
	// The underlying, adapted-to SourceManager
	sm SourceManager
	// Direction to sort the version list. False indicates sorting for upgrades;
	// true for downgrades.
	sortdown bool
	// Map of project root name to their available version list. This cache is
	// layered on top of the proper SourceManager's cache; the only difference
	// is that this keeps the versions sorted in the direction required by the
	// current solve run
	vlists map[ProjectName][]Version
}

func (c *smAdapter) getProjectInfo(pa ProjectAtom) (ProjectInfo, error) {
	return c.sm.GetProjectInfo(ProjectName(pa.Ident.netName()), pa.Version)
}

func (c *smAdapter) key(id ProjectIdentifier) ProjectName {
	k := ProjectName(id.NetworkName)
	if k == "" {
		k = id.LocalName
	}

	return k
}

func (c *smAdapter) listVersions(id ProjectIdentifier) ([]Version, error) {
	k := c.key(id)

	if vl, exists := c.vlists[k]; exists {
		return vl, nil
	}

	vl, err := c.sm.ListVersions(k)
	// TODO cache errors, too?
	if err != nil {
		return nil, err
	}

	if c.sortdown {
		sort.Sort(downgradeVersionSorter(vl))
	} else {
		sort.Sort(upgradeVersionSorter(vl))
	}

	c.vlists[k] = vl
	return vl, nil
}

func (c *smAdapter) repoExists(id ProjectIdentifier) (bool, error) {
	k := c.key(id)
	return c.sm.RepoExists(k)
}

func (c *smAdapter) vendorCodeExists(id ProjectIdentifier) (bool, error) {
	k := c.key(id)
	return c.sm.VendorCodeExists(k)
}

func (c *smAdapter) pairVersion(id ProjectIdentifier, v UnpairedVersion) PairedVersion {
	vl, err := c.listVersions(id)
	if err != nil {
		return nil
	}

	// doing it like this is a bit sloppy
	for _, v2 := range vl {
		if p, ok := v2.(PairedVersion); ok {
			if p.Matches(v) {
				return p
			}
		}
	}

	return nil
}

func (c *smAdapter) pairRevision(id ProjectIdentifier, r Revision) []Version {
	vl, err := c.listVersions(id)
	if err != nil {
		return nil
	}

	p := []Version{r}
	// doing it like this is a bit sloppy
	for _, v2 := range vl {
		if pv, ok := v2.(PairedVersion); ok {
			if pv.Matches(r) {
				p = append(p, pv)
			}
		}
	}

	return p
}

// matches performs a typical match check between the provided version and
// constraint. If that basic check fails and the provided version is incomplete
// (e.g. an unpaired version or bare revision), it will attempt to gather more
// information on one or the other and re-perform the comparison.
func (c *smAdapter) matches(id ProjectIdentifier, c2 Constraint, v Version) bool {
	if c2.Matches(v) {
		return true
	}

	// There's a wide field of possible ways that pairing might result in a
	// match. For each possible type of version, start by carving out all the
	// cases where the constraint would have provided an authoritative match
	// result.
	switch tv := v.(type) {
	case PairedVersion:
		switch tc := c2.(type) {
		case PairedVersion, Revision, noneConstraint:
			// These three would all have been authoritative matches
			return false
		case UnpairedVersion:
			// Only way paired and unpaired could match is if they share an
			// underlying rev
			pv := c.pairVersion(id, tc)
			if pv == nil {
				return false
			}
			return pv.Matches(v)
		case semverConstraint:
			// Have to check all the possible versions for that rev to see if
			// any match the semver constraint
			for _, pv := range c.pairRevision(id, tv.Underlying()) {
				if tc.Matches(pv) {
					return true
				}
			}
			return false
		}

	case Revision:
		switch tc := c2.(type) {
		case PairedVersion, Revision, noneConstraint:
			// These three would all have been authoritative matches
			return false
		case UnpairedVersion:
			// Only way paired and unpaired could match is if they share an
			// underlying rev
			pv := c.pairVersion(id, tc)
			if pv == nil {
				return false
			}
			return pv.Matches(v)
		case semverConstraint:
			// Have to check all the possible versions for the rev to see if
			// any match the semver constraint
			for _, pv := range c.pairRevision(id, tv) {
				if tc.Matches(pv) {
					return true
				}
			}
			return false
		}

	// UnpairedVersion as input has the most weird cases. It's also the one
	// we'll probably see the least
	case UnpairedVersion:
		switch tc := c2.(type) {
		case noneConstraint:
			// obviously
			return false
		case Revision, PairedVersion:
			// Easy case for both - just pair the uv and see if it matches the revision
			// constraint
			pv := c.pairVersion(id, tv)
			if pv == nil {
				return false
			}
			return tc.Matches(pv)
		case UnpairedVersion:
			// Both are unpaired versions. See if they share an underlying rev.
			pv := c.pairVersion(id, tv)
			if pv == nil {
				return false
			}

			pc := c.pairVersion(id, tc)
			if pc == nil {
				return false
			}
			return pc.Matches(pv)

		case semverConstraint:
			// semverConstraint can't ever match a rev, but we do need to check
			// if any other versions corresponding to this rev work.
			pv := c.pairVersion(id, tv)
			if pv == nil {
				return false
			}

			for _, ttv := range c.pairRevision(id, pv.Underlying()) {
				if c2.Matches(ttv) {
					return true
				}
			}
			return false
		}
	default:
		panic("unreachable")
	}
}

type upgradeVersionSorter []Version
type downgradeVersionSorter []Version

func (vs upgradeVersionSorter) Len() int {
	return len(vs)
}

func (vs upgradeVersionSorter) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}

func (vs downgradeVersionSorter) Len() int {
	return len(vs)
}

func (vs downgradeVersionSorter) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}

func (vs upgradeVersionSorter) Less(i, j int) bool {
	l, r := vs[i], vs[j]

	if tl, ispair := l.(versionPair); ispair {
		l = tl.v
	}
	if tr, ispair := r.(versionPair); ispair {
		r = tr.v
	}

	switch compareVersionType(l, r) {
	case -1:
		return true
	case 1:
		return false
	case 0:
		break
	default:
		panic("unreachable")
	}

	switch l.(type) {
	// For these, now nothing to do but alpha sort
	case Revision, branchVersion, plainVersion:
		return l.String() < r.String()
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lsv, rsv := l.(semVersion).sv, r.(semVersion).sv
	lpre, rpre := lsv.Prerelease() == "", rsv.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return lsv.GreaterThan(rsv)
}

func (vs downgradeVersionSorter) Less(i, j int) bool {
	l, r := vs[i], vs[j]

	if tl, ispair := l.(versionPair); ispair {
		l = tl.v
	}
	if tr, ispair := r.(versionPair); ispair {
		r = tr.v
	}

	switch compareVersionType(l, r) {
	case -1:
		return true
	case 1:
		return false
	case 0:
		break
	default:
		panic("unreachable")
	}

	switch l.(type) {
	// For these, now nothing to do but alpha
	case Revision, branchVersion, plainVersion:
		return l.String() < r.String()
	}

	// This ensures that pre-release versions are always sorted after ALL
	// full-release versions
	lsv, rsv := l.(semVersion).sv, r.(semVersion).sv
	lpre, rpre := lsv.Prerelease() == "", rsv.Prerelease() == ""
	if (lpre && !rpre) || (!lpre && rpre) {
		return lpre
	}
	return lsv.LessThan(rsv)
}
