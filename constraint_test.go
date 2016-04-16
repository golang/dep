package vsolver

import (
	"fmt"
	"testing"
)

// gu - helper func for stringifying what we assume is a VersionPair (otherwise
// will panic), but is given as a Constraint
func gu(v Constraint) string {
	return fmt.Sprintf("%q at rev %q", v, v.(PairedVersion).Underlying())
}

func TestBranchConstraintOps(t *testing.T) {
	v1 := NewBranch("master").(branchVersion)
	v2 := NewBranch("test").(branchVersion)
	none := none

	if v1.Matches(v2) {
		t.Errorf("%s should not match %s", v1, v2)
	}

	if v1.MatchesAny(v2) {
		t.Errorf("%s should not allow any matches when combined with %s", v1, v2)
	}

	if v1.Intersect(v2) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", v1, v2)
	}

	// Add rev to one
	snuffster := revision("snuffleupagus")
	v3 := v1.Is(snuffster).(versionPair)
	if v2.Matches(v3) {
		t.Errorf("%s should not match %s", v2, gu(v3))
	}
	if v3.Matches(v2) {
		t.Errorf("%s should not match %s", gu(v3), v2)
	}

	if v2.MatchesAny(v3) {
		t.Errorf("%s should not allow any matches when combined with %s", v2, gu(v3))
	}
	if v3.MatchesAny(v2) {
		t.Errorf("%s should not allow any matches when combined with %s", v2, gu(v3))
	}

	if v2.Intersect(v3) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", v2, gu(v3))
	}
	if v3.Intersect(v2) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v3), v2)
	}

	// Add different rev to the other
	v4 := v2.Is(revision("cookie monster")).(versionPair)
	if v4.Matches(v3) {
		t.Errorf("%s should not match %s", gu(v4), gu(v3))
	}
	if v3.Matches(v4) {
		t.Errorf("%s should not match %s", gu(v3), gu(v4))
	}

	if v4.MatchesAny(v3) {
		t.Errorf("%s should not allow any matches when combined with %s", gu(v4), gu(v3))
	}
	if v3.MatchesAny(v4) {
		t.Errorf("%s should not allow any matches when combined with %s", gu(v4), gu(v3))
	}

	if v4.Intersect(v3) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v4), gu(v3))
	}
	if v3.Intersect(v4) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v3), gu(v4))
	}

	// Now add same rev to different branches
	// TODO this might not actually be a good idea, when you consider the
	// semantics of floating versions...matching on an underlying rev might be
	// nice in the short term, but it's probably shit most of the time
	v5 := v2.Is(revision("snuffleupagus")).(versionPair)
	if !v5.Matches(v3) {
		t.Errorf("%s should match %s", gu(v5), gu(v3))
	}
	if !v3.Matches(v5) {
		t.Errorf("%s should match %s", gu(v3), gu(v5))
	}

	if !v5.MatchesAny(v3) {
		t.Errorf("%s should allow some matches when combined with %s", gu(v5), gu(v3))
	}
	if !v3.MatchesAny(v5) {
		t.Errorf("%s should allow some matches when combined with %s", gu(v5), gu(v3))
	}

	if v5.Intersect(v3) != snuffster {
		t.Errorf("Intersection of %s with %s should return underlying rev", gu(v5), gu(v3))
	}
	if v3.Intersect(v5) != snuffster {
		t.Errorf("Intersection of %s with %s should return underlying rev", gu(v3), gu(v5))
	}

	// Set up for cross-type constraint ops
	cookie := revision("cookie monster")
	o1 := NewVersion("master").(plainVersion)
	o2 := NewVersion("1.0.0").(semVersion)
	o3 := o1.Is(cookie).(versionPair)
	o4 := o2.Is(cookie).(versionPair)
	v6 := v1.Is(cookie).(versionPair)

	if v1.Matches(o1) {
		t.Errorf("%s (branch) should not match %s (version) across types", v1, o1)
	}

	if v1.MatchesAny(o1) {
		t.Errorf("%s (branch) should not allow any matches when combined with %s (version)", v1, o1)
	}

	if v1.Intersect(o1) != none {
		t.Errorf("Intersection of %s (branch) with %s (version) should result in empty set", v1, o1)
	}

	if v1.Matches(o2) {
		t.Errorf("%s (branch) should not match %s (semver) across types", v1, o2)
	}

	if v1.MatchesAny(o2) {
		t.Errorf("%s (branch) should not allow any matches when combined with %s (semver)", v1, o2)
	}

	if v1.Intersect(o2) != none {
		t.Errorf("Intersection of %s (branch) with %s (semver) should result in empty set", v1, o2)
	}

	if v1.Matches(o3) {
		t.Errorf("%s (branch) should not match %s (version) across types", v1, gu(o3))
	}

	if v1.MatchesAny(o3) {
		t.Errorf("%s (branch) should not allow any matches when combined with %s (version)", v1, gu(o3))
	}

	if v1.Intersect(o3) != none {
		t.Errorf("Intersection of %s (branch) with %s (version) should result in empty set", v1, gu(o3))
	}

	if v1.Matches(o4) {
		t.Errorf("%s (branch) should not match %s (semver) across types", v1, gu(o4))
	}

	if v1.MatchesAny(o4) {
		t.Errorf("%s (branch) should not allow any matches when combined with %s (semver)", v1, gu(o4))
	}

	if v1.Intersect(o4) != none {
		t.Errorf("Intersection of %s (branch) with %s (semver) should result in empty set", v1, gu(o4))
	}

	if !v6.Matches(o3) {
		t.Errorf("%s (branch) should match %s (version) across types due to shared rev", gu(v6), gu(o3))
	}

	if !v6.MatchesAny(o3) {
		t.Errorf("%s (branch) should allow some matches when combined with %s (version) across types due to shared rev", gu(v6), gu(o3))
	}

	if v6.Intersect(o3) != cookie {
		t.Errorf("Intersection of %s (branch) with %s (version) should return shared underlying rev", gu(v6), gu(o3))
	}

	if !v6.Matches(o4) {
		t.Errorf("%s (branch) should match %s (version) across types due to shared rev", gu(v6), gu(o4))
	}

	if !v6.MatchesAny(o4) {
		t.Errorf("%s (branch) should allow some matches when combined with %s (version) across types due to shared rev", gu(v6), gu(o4))
	}

	if v6.Intersect(o4) != cookie {
		t.Errorf("Intersection of %s (branch) with %s (version) should return shared underlying rev", gu(v6), gu(o4))
	}
}

func TestVersionConstraintOps(t *testing.T) {
	v1 := NewVersion("ab123").(plainVersion)
	v2 := NewVersion("b2a13").(plainVersion)
	none := none

	if v1.Matches(v2) {
		t.Errorf("%s should not match %s", v1, v2)
	}

	if v1.MatchesAny(v2) {
		t.Errorf("%s should not allow any matches when combined with %s", v1, v2)
	}

	if v1.Intersect(v2) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", v1, v2)
	}

	// Add rev to one
	snuffster := revision("snuffleupagus")
	v3 := v1.Is(snuffster).(versionPair)
	if v2.Matches(v3) {
		t.Errorf("%s should not match %s", v2, gu(v3))
	}
	if v3.Matches(v2) {
		t.Errorf("%s should not match %s", gu(v3), v2)
	}

	if v2.MatchesAny(v3) {
		t.Errorf("%s should not allow any matches when combined with %s", v2, gu(v3))
	}
	if v3.MatchesAny(v2) {
		t.Errorf("%s should not allow any matches when combined with %s", v2, gu(v3))
	}

	if v2.Intersect(v3) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", v2, gu(v3))
	}
	if v3.Intersect(v2) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v3), v2)
	}

	// Add different rev to the other
	v4 := v2.Is(revision("cookie monster")).(versionPair)
	if v4.Matches(v3) {
		t.Errorf("%s should not match %s", gu(v4), gu(v3))
	}
	if v3.Matches(v4) {
		t.Errorf("%s should not match %s", gu(v3), gu(v4))
	}

	if v4.MatchesAny(v3) {
		t.Errorf("%s should not allow any matches when combined with %s", gu(v4), gu(v3))
	}
	if v3.MatchesAny(v4) {
		t.Errorf("%s should not allow any matches when combined with %s", gu(v4), gu(v3))
	}

	if v4.Intersect(v3) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v4), gu(v3))
	}
	if v3.Intersect(v4) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v3), gu(v4))
	}

	// Now add same rev to different versions, and things should line up
	v5 := v2.Is(revision("snuffleupagus")).(versionPair)
	if !v5.Matches(v3) {
		t.Errorf("%s should match %s", gu(v5), gu(v3))
	}
	if !v3.Matches(v5) {
		t.Errorf("%s should match %s", gu(v3), gu(v5))
	}

	if !v5.MatchesAny(v3) {
		t.Errorf("%s should allow some matches when combined with %s", gu(v5), gu(v3))
	}
	if !v3.MatchesAny(v5) {
		t.Errorf("%s should allow some matches when combined with %s", gu(v5), gu(v3))
	}

	if v5.Intersect(v3) != snuffster {
		t.Errorf("Intersection of %s with %s should return underlying rev", gu(v5), gu(v3))
	}
	if v3.Intersect(v5) != snuffster {
		t.Errorf("Intersection of %s with %s should return underlying rev", gu(v3), gu(v5))
	}

	// Set up for cross-type constraint ops
	cookie := revision("cookie monster")
	o1 := NewBranch("master").(branchVersion)
	o2 := NewVersion("1.0.0").(semVersion)
	o3 := o1.Is(cookie).(versionPair)
	o4 := o2.Is(cookie).(versionPair)
	v6 := v1.Is(cookie).(versionPair)

	if v1.Matches(o1) {
		t.Errorf("%s (version) should not match %s (branch) across types", v1, o1)
	}

	if v1.MatchesAny(o1) {
		t.Errorf("%s (version) should not allow any matches when combined with %s (branch)", v1, o1)
	}

	if v1.Intersect(o1) != none {
		t.Errorf("Intersection of %s (version) with %s (branch) should result in empty set", v1, o1)
	}

	if v1.Matches(o2) {
		t.Errorf("%s (version) should not match %s (semver) across types", v1, o2)
	}

	if v1.MatchesAny(o2) {
		t.Errorf("%s (version) should not allow any matches when combined with %s (semver)", v1, o2)
	}

	if v1.Intersect(o2) != none {
		t.Errorf("Intersection of %s (version) with %s (semver) should result in empty set", v1, o2)
	}

	if v1.Matches(o3) {
		t.Errorf("%s (version) should not match %s (branch) across types", v1, gu(o3))
	}

	if v1.MatchesAny(o3) {
		t.Errorf("%s (version) should not allow any matches when combined with %s (branch)", v1, gu(o3))
	}

	if v1.Intersect(o3) != none {
		t.Errorf("Intersection of %s (version) with %s (branch) should result in empty set", v1, gu(o3))
	}

	if v1.Matches(o4) {
		t.Errorf("%s (version) should not match %s (semver) across types", v1, gu(o4))
	}

	if v1.MatchesAny(o4) {
		t.Errorf("%s (version) should not allow any matches when combined with %s (semver)", v1, gu(o4))
	}

	if v1.Intersect(o4) != none {
		t.Errorf("Intersection of %s (version) with %s (semver) should result in empty set", v1, gu(o4))
	}

	if !v6.Matches(o3) {
		t.Errorf("%s (version) should match %s (branch) across types due to shared rev", gu(v6), gu(o3))
	}

	if !v6.MatchesAny(o3) {
		t.Errorf("%s (version) should allow some matches when combined with %s (branch) across types due to shared rev", gu(v6), gu(o3))
	}

	if v6.Intersect(o3) != cookie {
		t.Errorf("Intersection of %s (version) with %s (branch) should return shared underlying rev", gu(v6), gu(o3))
	}

	if !v6.Matches(o4) {
		t.Errorf("%s (version) should match %s (branch) across types due to shared rev", gu(v6), gu(o4))
	}

	if !v6.MatchesAny(o4) {
		t.Errorf("%s (version) should allow some matches when combined with %s (branch) across types due to shared rev", gu(v6), gu(o4))
	}

	if v6.Intersect(o4) != cookie {
		t.Errorf("Intersection of %s (version) with %s (branch) should return shared underlying rev", gu(v6), gu(o4))
	}
}

func TestSemverVersionConstraintOps(t *testing.T) {
	v1 := NewVersion("1.0.0").(semVersion)
	v2 := NewVersion("2.0.0").(semVersion)
	none := none

	if v1.Matches(v2) {
		t.Errorf("%s should not match %s", v1, v2)
	}

	if v1.MatchesAny(v2) {
		t.Errorf("%s should not allow any matches when combined with %s", v1, v2)
	}

	if v1.Intersect(v2) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", v1, v2)
	}

	// Add rev to one
	snuffster := revision("snuffleupagus")
	v3 := v1.Is(snuffster).(versionPair)
	if v2.Matches(v3) {
		t.Errorf("%s should not match %s", v2, gu(v3))
	}
	if v3.Matches(v2) {
		t.Errorf("%s should not match %s", gu(v3), v2)
	}

	if v2.MatchesAny(v3) {
		t.Errorf("%s should not allow any matches when combined with %s", v2, gu(v3))
	}
	if v3.MatchesAny(v2) {
		t.Errorf("%s should not allow any matches when combined with %s", v2, gu(v3))
	}

	if v2.Intersect(v3) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", v2, gu(v3))
	}
	if v3.Intersect(v2) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v3), v2)
	}

	// Add different rev to the other
	v4 := v2.Is(revision("cookie monster")).(versionPair)
	if v4.Matches(v3) {
		t.Errorf("%s should not match %s", gu(v4), gu(v3))
	}
	if v3.Matches(v4) {
		t.Errorf("%s should not match %s", gu(v3), gu(v4))
	}

	if v4.MatchesAny(v3) {
		t.Errorf("%s should not allow any matches when combined with %s", gu(v4), gu(v3))
	}
	if v3.MatchesAny(v4) {
		t.Errorf("%s should not allow any matches when combined with %s", gu(v4), gu(v3))
	}

	if v4.Intersect(v3) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v4), gu(v3))
	}
	if v3.Intersect(v4) != none {
		t.Errorf("Intersection of %s with %s should result in empty set", gu(v3), gu(v4))
	}

	// Now add same rev to different versions, and things should line up
	v5 := v2.Is(revision("snuffleupagus")).(versionPair)
	if !v5.Matches(v3) {
		t.Errorf("%s should match %s", gu(v5), gu(v3))
	}
	if !v3.Matches(v5) {
		t.Errorf("%s should match %s", gu(v3), gu(v5))
	}

	if !v5.MatchesAny(v3) {
		t.Errorf("%s should allow some matches when combined with %s", gu(v5), gu(v3))
	}
	if !v3.MatchesAny(v5) {
		t.Errorf("%s should allow some matches when combined with %s", gu(v5), gu(v3))
	}

	if v5.Intersect(v3) != snuffster {
		t.Errorf("Intersection of %s with %s should return underlying rev", gu(v5), gu(v3))
	}
	if v3.Intersect(v5) != snuffster {
		t.Errorf("Intersection of %s with %s should return underlying rev", gu(v3), gu(v5))
	}

	// Set up for cross-type constraint ops
	cookie := revision("cookie monster")
	o1 := NewBranch("master").(branchVersion)
	o2 := NewVersion("ab123").(plainVersion)
	o3 := o1.Is(cookie).(versionPair)
	o4 := o2.Is(cookie).(versionPair)
	v6 := v1.Is(cookie).(versionPair)

	if v1.Matches(o1) {
		t.Errorf("%s (semver) should not match %s (branch) across types", v1, o1)
	}

	if v1.MatchesAny(o1) {
		t.Errorf("%s (semver) should not allow any matches when combined with %s (branch)", v1, o1)
	}

	if v1.Intersect(o1) != none {
		t.Errorf("Intersection of %s (semver) with %s (branch) should result in empty set", v1, o1)
	}

	if v1.Matches(o2) {
		t.Errorf("%s (semver) should not match %s (version) across types", v1, o2)
	}

	if v1.MatchesAny(o2) {
		t.Errorf("%s (semver) should not allow any matches when combined with %s (version)", v1, o2)
	}

	if v1.Intersect(o2) != none {
		t.Errorf("Intersection of %s (semver) with %s (version) should result in empty set", v1, o2)
	}

	if v1.Matches(o3) {
		t.Errorf("%s (semver) should not match %s (branch) across types", v1, gu(o3))
	}

	if v1.MatchesAny(o3) {
		t.Errorf("%s (semver) should not allow any matches when combined with %s (branch)", v1, gu(o3))
	}

	if v1.Intersect(o3) != none {
		t.Errorf("Intersection of %s (semver) with %s (branch) should result in empty set", v1, gu(o3))
	}

	if v1.Matches(o4) {
		t.Errorf("%s (semver) should not match %s (version) across types", v1, gu(o4))
	}

	if v1.MatchesAny(o4) {
		t.Errorf("%s (semver) should not allow any matches when combined with %s (version)", v1, gu(o4))
	}

	if v1.Intersect(o4) != none {
		t.Errorf("Intersection of %s (semver) with %s (version) should result in empty set", v1, gu(o4))
	}

	if !v6.Matches(o3) {
		t.Errorf("%s (semver) should match %s (branch) across types due to shared rev", gu(v6), gu(o3))
	}

	if !v6.MatchesAny(o3) {
		t.Errorf("%s (semver) should allow some matches when combined with %s (branch) across types due to shared rev", gu(v6), gu(o3))
	}

	if v6.Intersect(o3) != cookie {
		t.Errorf("Intersection of %s (semver) with %s (branch) should return shared underlying rev", gu(v6), gu(o3))
	}

	if !v6.Matches(o4) {
		t.Errorf("%s (semver) should match %s (branch) across types due to shared rev", gu(v6), gu(o4))
	}

	if !v6.MatchesAny(o4) {
		t.Errorf("%s (semver) should allow some matches when combined with %s (branch) across types due to shared rev", gu(v6), gu(o4))
	}

	if v6.Intersect(o4) != cookie {
		t.Errorf("Intersection of %s (semver) with %s (branch) should return shared underlying rev", gu(v6), gu(o4))
	}
}

// The other test is about the semverVersion, this is about semverConstraint
func TestSemverConstraintOps(t *testing.T) {
	v1 := NewBranch("master").(branchVersion)
	v2 := NewVersion("ab123").(plainVersion)
	v3 := NewVersion("1.0.0").(semVersion)

	fozzie := revision("fozzie bear")
	v4 := v1.Is(fozzie).(versionPair)
	v5 := v2.Is(fozzie).(versionPair)
	v6 := v3.Is(fozzie).(versionPair)

	c1, err := NewConstraint(">= 1.0.0", SemverConstraint)
	if err != nil {
		t.Errorf("Failed to create constraint: %s", err)
		t.FailNow()
	}

	if c1.Matches(v1) {
		t.Errorf("Semver constraint should not match simple branch")
	}
	if c1.Matches(v2) {
		t.Errorf("Semver constraint should not match simple version")
	}
	if !c1.Matches(v3) {
		t.Errorf("Semver constraint should match a simple semver version in its range")
	}
	if c1.Matches(v4) {
		t.Errorf("Semver constraint should not match paired branch")
	}
	if c1.Matches(v5) {
		t.Errorf("Semver constraint should not match paired version")
	}
	if !c1.Matches(v6) {
		t.Errorf("Semver constraint should match a paired semver version in its range")
	}

	if c1.MatchesAny(v1) {
		t.Errorf("Semver constraint should not allow any when intersected with simple branch")
	}
	if c1.MatchesAny(v2) {
		t.Errorf("Semver constraint should not allow any when intersected with simple version")
	}
	if !c1.MatchesAny(v3) {
		t.Errorf("Semver constraint should allow some when intersected with a simple semver version in its range")
	}
	if c1.MatchesAny(v4) {
		t.Errorf("Semver constraint should not allow any when intersected with paired branch")
	}
	if c1.MatchesAny(v5) {
		t.Errorf("Semver constraint should not allow any when intersected with paired version")
	}
	if !c1.MatchesAny(v6) {
		t.Errorf("Semver constraint should allow some when intersected with a paired semver version in its range")
	}

	if c1.Intersect(v1) != none {
		t.Errorf("Semver constraint should return none when intersected with a simple branch")
	}
	if c1.Intersect(v2) != none {
		t.Errorf("Semver constraint should return none when intersected with a simple version")
	}
	if c1.Intersect(v3) != v3 {
		t.Errorf("Semver constraint should return input when intersected with a simple semver version in its range")
	}
	if c1.Intersect(v4) != none {
		t.Errorf("Semver constraint should return none when intersected with a paired branch")
	}
	if c1.Intersect(v5) != none {
		t.Errorf("Semver constraint should return none when intersected with a paired version")
	}
	if c1.Intersect(v6) != v6 {
		t.Errorf("Semver constraint should return input when intersected with a paired semver version in its range")
	}
}
