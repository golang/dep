package gps

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

	if !v1.MatchesAny(any) {
		t.Errorf("Branches should always match the any constraint")
	}
	if v1.Intersect(any) != v1 {
		t.Errorf("Branches should always return self when intersecting the any constraint, but got %s", v1.Intersect(any))
	}

	if v1.MatchesAny(none) {
		t.Errorf("Branches should never match the none constraint")
	}
	if v1.Intersect(none) != none {
		t.Errorf("Branches should always return none when intersecting the none constraint, but got %s", v1.Intersect(none))
	}

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
	snuffster := Revision("snuffleupagus")
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
	v4 := v2.Is(Revision("cookie monster")).(versionPair)
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
	// TODO(sdboyer) this might not actually be a good idea, when you consider the
	// semantics of floating versions...matching on an underlying rev might be
	// nice in the short term, but it's probably shit most of the time
	v5 := v2.Is(Revision("snuffleupagus")).(versionPair)
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
	cookie := Revision("cookie monster")
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

	if !v1.MatchesAny(any) {
		t.Errorf("Versions should always match the any constraint")
	}
	if v1.Intersect(any) != v1 {
		t.Errorf("Versions should always return self when intersecting the any constraint, but got %s", v1.Intersect(any))
	}

	if v1.MatchesAny(none) {
		t.Errorf("Versions should never match the none constraint")
	}
	if v1.Intersect(none) != none {
		t.Errorf("Versions should always return none when intersecting the none constraint, but got %s", v1.Intersect(none))
	}

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
	snuffster := Revision("snuffleupagus")
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
	v4 := v2.Is(Revision("cookie monster")).(versionPair)
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
	v5 := v2.Is(Revision("snuffleupagus")).(versionPair)
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
	cookie := Revision("cookie monster")
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

	if !v1.MatchesAny(any) {
		t.Errorf("Semvers should always match the any constraint")
	}
	if v1.Intersect(any) != v1 {
		t.Errorf("Semvers should always return self when intersecting the any constraint, but got %s", v1.Intersect(any))
	}

	if v1.MatchesAny(none) {
		t.Errorf("Semvers should never match the none constraint")
	}
	if v1.Intersect(none) != none {
		t.Errorf("Semvers should always return none when intersecting the none constraint, but got %s", v1.Intersect(none))
	}

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
	snuffster := Revision("snuffleupagus")
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
	v4 := v2.Is(Revision("cookie monster")).(versionPair)
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
	v5 := v2.Is(Revision("snuffleupagus")).(versionPair)
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
	cookie := Revision("cookie monster")
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

	// Regression check - make sure that semVersion -> semverConstraint works
	// the same as verified in the other test
	c1, _ := NewSemverConstraint("=1.0.0")
	if !v1.MatchesAny(c1) {
		t.Errorf("%s (semver) should allow some matches - itself - when combined with an equivalent semverConstraint", gu(v1))
	}
	if v1.Intersect(c1) != v1 {
		t.Errorf("Intersection of %s (semver) with equivalent semver constraint should return self, got %s", gu(v1), v1.Intersect(c1))
	}

	if !v6.MatchesAny(c1) {
		t.Errorf("%s (semver pair) should allow some matches - itself - when combined with an equivalent semverConstraint", gu(v6))
	}
	if v6.Intersect(c1) != v6 {
		t.Errorf("Intersection of %s (semver pair) with equivalent semver constraint should return self, got %s", gu(v6), v6.Intersect(c1))
	}

}

// The other test is about the semverVersion, this is about semverConstraint
func TestSemverConstraintOps(t *testing.T) {
	v1 := NewBranch("master").(branchVersion)
	v2 := NewVersion("ab123").(plainVersion)
	v3 := NewVersion("1.0.0").(semVersion)

	fozzie := Revision("fozzie bear")
	v4 := v1.Is(fozzie).(versionPair)
	v5 := v2.Is(fozzie).(versionPair)
	v6 := v3.Is(fozzie).(versionPair)

	// TODO(sdboyer) we can't use the same range as below b/c semver.rangeConstraint is
	// still an incomparable type
	c1, err := NewSemverConstraint("=1.0.0")
	if err != nil {
		t.Fatalf("Failed to create constraint: %s", err)
	}

	if !c1.MatchesAny(any) {
		t.Errorf("Semver constraints should always match the any constraint")
	}
	if c1.Intersect(any) != c1 {
		t.Errorf("Semver constraints should always return self when intersecting the any constraint, but got %s", c1.Intersect(any))
	}

	if c1.MatchesAny(none) {
		t.Errorf("Semver constraints should never match the none constraint")
	}
	if c1.Intersect(none) != none {
		t.Errorf("Semver constraints should always return none when intersecting the none constraint, but got %s", c1.Intersect(none))
	}

	c1, err = NewSemverConstraint(">= 1.0.0")
	if err != nil {
		t.Fatalf("Failed to create constraint: %s", err)
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

// Test that certain types of cross-version comparisons work when they are
// expressed as a version union (but that others don't).
func TestVersionUnion(t *testing.T) {
	rev := Revision("flooboofoobooo")
	v1 := NewBranch("master")
	v2 := NewBranch("test")
	v3 := NewVersion("1.0.0").Is(rev)
	v4 := NewVersion("1.0.1")
	v5 := NewVersion("v2.0.5").Is(Revision("notamatch"))

	uv1 := versionTypeUnion{v1, v4, rev}
	uv2 := versionTypeUnion{v2, v3}

	if uv1.MatchesAny(none) {
		t.Errorf("Union can't match none")
	}
	if none.MatchesAny(uv1) {
		t.Errorf("Union can't match none")
	}

	if !uv1.MatchesAny(any) {
		t.Errorf("Union must match any")
	}
	if !any.MatchesAny(uv1) {
		t.Errorf("Union must match any")
	}

	// Basic matching
	if !uv1.Matches(v4) {
		t.Errorf("Union should match on branch to branch")
	}
	if !v4.Matches(uv1) {
		t.Errorf("Union should reverse-match on branch to branch")
	}

	if !uv1.Matches(v3) {
		t.Errorf("Union should match on rev to paired rev")
	}
	if !v3.Matches(uv1) {
		t.Errorf("Union should reverse-match on rev to paired rev")
	}

	if uv1.Matches(v2) {
		t.Errorf("Union should not match on anything in disjoint unpaired")
	}
	if v2.Matches(uv1) {
		t.Errorf("Union should not reverse-match on anything in disjoint unpaired")
	}

	if uv1.Matches(v5) {
		t.Errorf("Union should not match on anything in disjoint pair")
	}
	if v5.Matches(uv1) {
		t.Errorf("Union should not reverse-match on anything in disjoint pair")
	}

	if !uv1.Matches(uv2) {
		t.Errorf("Union should succeed on matching comparison to other union with some overlap")
	}

	// MatchesAny - repeat Matches for safety, but add more, too
	if !uv1.MatchesAny(v4) {
		t.Errorf("Union should match on branch to branch")
	}
	if !v4.MatchesAny(uv1) {
		t.Errorf("Union should reverse-match on branch to branch")
	}

	if !uv1.MatchesAny(v3) {
		t.Errorf("Union should match on rev to paired rev")
	}
	if !v3.MatchesAny(uv1) {
		t.Errorf("Union should reverse-match on rev to paired rev")
	}

	if uv1.MatchesAny(v2) {
		t.Errorf("Union should not match on anything in disjoint unpaired")
	}
	if v2.MatchesAny(uv1) {
		t.Errorf("Union should not reverse-match on anything in disjoint unpaired")
	}

	if uv1.MatchesAny(v5) {
		t.Errorf("Union should not match on anything in disjoint pair")
	}
	if v5.MatchesAny(uv1) {
		t.Errorf("Union should not reverse-match on anything in disjoint pair")
	}

	c1, _ := NewSemverConstraint("~1.0.0")
	c2, _ := NewSemverConstraint("~2.0.0")
	if !uv1.MatchesAny(c1) {
		t.Errorf("Union should have some overlap due to containing 1.0.1 version")
	}
	if !c1.MatchesAny(uv1) {
		t.Errorf("Union should have some overlap due to containing 1.0.1 version")
	}

	if uv1.MatchesAny(c2) {
		t.Errorf("Union should have no overlap with ~2.0.0 semver range")
	}
	if c2.MatchesAny(uv1) {
		t.Errorf("Union should have no overlap with ~2.0.0 semver range")
	}

	if !uv1.MatchesAny(uv2) {
		t.Errorf("Union should succeed on MatchAny against other union with some overlap")
	}

	// Intersect - repeat all previous
	if uv1.Intersect(v4) != v4 {
		t.Errorf("Union intersection on contained version should return that version")
	}
	if v4.Intersect(uv1) != v4 {
		t.Errorf("Union reverse-intersection on contained version should return that version")
	}

	if uv1.Intersect(v3) != rev {
		t.Errorf("Union intersection on paired version w/matching rev should return rev, got %s", uv1.Intersect(v3))
	}
	if v3.Intersect(uv1) != rev {
		t.Errorf("Union reverse-intersection on paired version w/matching rev should return rev, got %s", v3.Intersect(uv1))
	}

	if uv1.Intersect(v2) != none {
		t.Errorf("Union should not intersect with anything in disjoint unpaired")
	}
	if v2.Intersect(uv1) != none {
		t.Errorf("Union should not reverse-intersect with anything in disjoint unpaired")
	}

	if uv1.Intersect(v5) != none {
		t.Errorf("Union should not intersect with anything in disjoint pair")
	}
	if v5.Intersect(uv1) != none {
		t.Errorf("Union should not reverse-intersect with anything in disjoint pair")
	}

	if uv1.Intersect(c1) != v4 {
		t.Errorf("Union intersecting with semver range should return 1.0.1 version, got %s", uv1.Intersect(c1))
	}
	if c1.Intersect(uv1) != v4 {
		t.Errorf("Union reverse-intersecting with semver range should return 1.0.1 version, got %s", c1.Intersect(uv1))
	}

	if uv1.Intersect(c2) != none {
		t.Errorf("Union intersecting with non-overlapping semver range should return none, got %s", uv1.Intersect(c2))
	}
	if c2.Intersect(uv1) != none {
		t.Errorf("Union reverse-intersecting with non-overlapping semver range should return none, got %s", uv1.Intersect(c2))
	}

	if uv1.Intersect(uv2) != rev {
		t.Errorf("Unions should intersect down to rev, but got %s", uv1.Intersect(uv2))
	}
}

func TestVersionUnionPanicOnType(t *testing.T) {
	// versionTypeUnions need to panic if Type() gets called
	defer func() {
		if err := recover(); err == nil {
			t.Error("versionTypeUnion did not panic on Type() call")
		}
	}()
	_ = versionTypeUnion{}.Type()
}

func TestVersionUnionPanicOnString(t *testing.T) {
	// versionStringUnions need to panic if String() gets called
	defer func() {
		if err := recover(); err == nil {
			t.Error("versionStringUnion did not panic on String() call")
		}
	}()
	_ = versionTypeUnion{}.String()
}

func TestTypedConstraintString(t *testing.T) {
	// Also tests typedVersionString(), as this nests down into that
	rev := Revision("flooboofoobooo")
	v1 := NewBranch("master")
	v2 := NewBranch("test").Is(rev)
	v3 := NewVersion("1.0.1")
	v4 := NewVersion("v2.0.5")
	v5 := NewVersion("2.0.5.2")

	table := []struct {
		in  Constraint
		out string
	}{
		{
			in:  anyConstraint{},
			out: "any-*",
		},
		{
			in:  noneConstraint{},
			out: "none-",
		},
		{
			in:  mkSVC("^1.0.0"),
			out: "svc-^1.0.0",
		},
		{
			in:  v1,
			out: "b-master",
		},
		{
			in:  v2,
			out: "b-test-r-" + string(rev),
		},
		{
			in:  v3,
			out: "sv-1.0.1",
		},
		{
			in:  v4,
			out: "sv-v2.0.5",
		},
		{
			in:  v5,
			out: "pv-2.0.5.2",
		},
	}

	for _, fix := range table {
		got := fix.in.typedString()
		if got != fix.out {
			t.Errorf("Typed string for %v (%T) was not expected %q; got %q", fix.in, fix.in, fix.out, got)
		}
	}
}
