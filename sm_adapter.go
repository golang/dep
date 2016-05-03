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
	return c.sm.GetProjectInfo(ProjectName(pa.Name.netName()), pa.Version)
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
