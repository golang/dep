package gps

import "testing"

func TestVersionSorts(t *testing.T) {
	rev := Revision("flooboofoobooo")
	v1 := NewBranch("master").Is(rev)
	v2 := NewBranch("test").Is(rev)
	v3 := NewVersion("1.0.0").Is(rev)
	v4 := NewVersion("1.0.1")
	v5 := NewVersion("v2.0.5")
	v6 := NewVersion("2.0.5.2")
	v7 := newDefaultBranch("unwrapped")
	v8 := NewVersion("20.0.5.2")
	v9 := NewVersion("v1.5.5-beta.4")
	v10 := NewVersion("v3.0.1-alpha.1")

	start := []Version{
		v1,
		v2,
		v3,
		v4,
		v5,
		v6,
		v7,
		v8,
		v9,
		v10,
		rev,
	}

	down := make([]Version, len(start))
	copy(down, start)
	up := make([]Version, len(start))
	copy(up, start)

	edown := []Version{
		v3, v4, v5, // semvers
		v9, v10, // prerelease semver
		v7, v1, v2, // floating/branches
		v6, v8, // plain versions
		rev, // revs
	}

	eup := []Version{
		v5, v4, v3, // semvers
		v10, v9, // prerelease semver
		v7, v1, v2, // floating/branches
		v6, v8, // plain versions
		rev, // revs
	}

	SortForUpgrade(up)
	var wrong []int
	for k, v := range up {
		if eup[k] != v {
			wrong = append(wrong, k)
			t.Errorf("Expected version %s in position %v on upgrade sort, but got %s", eup[k], k, v)
		}
	}
	if len(wrong) > 0 {
		// Just helps with readability a bit
		t.Errorf("Upgrade sort positions with wrong versions: %v", wrong)
	}

	SortForDowngrade(down)
	wrong = wrong[:0]
	for k, v := range down {
		if edown[k] != v {
			wrong = append(wrong, k)
			t.Errorf("Expected version %s in position %v on downgrade sort, but got %s", edown[k], k, v)
		}
	}
	if len(wrong) > 0 {
		// Just helps with readability a bit
		t.Errorf("Downgrade sort positions with wrong versions: %v", wrong)
	}

	// Now make sure we sort back the other way correctly...just because
	SortForUpgrade(down)
	wrong = wrong[:0]
	for k, v := range down {
		if eup[k] != v {
			wrong = append(wrong, k)
			t.Errorf("Expected version %s in position %v on down-then-upgrade sort, but got %s", eup[k], k, v)
		}
	}
	if len(wrong) > 0 {
		// Just helps with readability a bit
		t.Errorf("Down-then-upgrade sort positions with wrong versions: %v", wrong)
	}

	// Now make sure we sort back the other way correctly...just because
	SortForDowngrade(up)
	wrong = wrong[:0]
	for k, v := range up {
		if edown[k] != v {
			wrong = append(wrong, k)
			t.Errorf("Expected version %s in position %v on up-then-downgrade sort, but got %s", edown[k], k, v)
		}
	}
	if len(wrong) > 0 {
		// Just helps with readability a bit
		t.Errorf("Up-then-downgrade sort positions with wrong versions: %v", wrong)
	}
}
