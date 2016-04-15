package vsolver

import (
	"fmt"
	"testing"
)

// gu - helper func for stringifying what we assume is a VersionPair (otherwise
// will panic), but is given as a Constraint
func gu(v Constraint) string {
	return fmt.Sprintf("%q at rev %q", v, v.(VersionPair).Underlying())
}

func TestBranchConstraintOps(t *testing.T) {
	v1 := NewFloatingVersion("master").(floatingVersion)
	v2 := NewFloatingVersion("test").(floatingVersion)
	none := noneConstraint{}

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
	// TODO this might not actually be a good idea, when you consider the
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
}

func TestVersionConstraintOps(t *testing.T) {
	v1 := NewVersion("ab123").(plainVersion)
	v2 := NewVersion("b2a13").(plainVersion)
	none := noneConstraint{}

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
}

func TestSemverVersionConstraintOps(t *testing.T) {
	v1 := NewVersion("1.0.0").(semverVersion)
	v2 := NewVersion("2.0.0").(semverVersion)
	none := noneConstraint{}

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
}
