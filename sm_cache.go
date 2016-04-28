package vsolver

import "sort"

type smcache struct {
	// The decorated/underlying SourceManager
	sm SourceManager
	// Direction to sort the version list. True indicates sorting for upgrades;
	// false for downgrades.
	sortup bool
	// Map of project root name to their available version list. This cache is
	// layered on top of the proper SourceManager's cache; the only difference
	// is that this keeps the versions sorted in the direction required by the
	// current solve run
	vlists map[ProjectName][]Version
}

// ensure interface fulfillment
var _ SourceManager = &smcache{}

func (c *smcache) GetProjectInfo(pa ProjectAtom) (ProjectInfo, error) {
	return c.sm.GetProjectInfo(pa)
}

func (c *smcache) ListVersions(n ProjectName) ([]Version, error) {
	if vl, exists := c.vlists[n]; exists {
		return vl, nil
	}

	vl, err := c.sm.ListVersions(n)
	// TODO cache errors, too?
	if err != nil {
		return nil, err
	}

	if c.sortup {
		sort.Sort(upgradeVersionSorter(vl))
	} else {
		sort.Sort(downgradeVersionSorter(vl))
	}

	c.vlists[n] = vl
	return vl, nil
}

func (c *smcache) RepoExists(n ProjectName) (bool, error) {
	return c.sm.RepoExists(n)
}

func (c *smcache) VendorCodeExists(n ProjectName) (bool, error) {
	return c.sm.VendorCodeExists(n)
}

func (c *smcache) ExportAtomTo(ProjectAtom, string) error {
	// No reason this should ever be called, as smcache's use is strictly
	// solver-internal and the solver never exports atoms
	panic("*smcache should never be asked to export an atom")
}

func (c *smcache) Release() {
	c.sm.Release()
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
