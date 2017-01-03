package gps

import (
	"reflect"
	"testing"
)

func TestLockedProjectSorting(t *testing.T) {
	// version doesn't matter here
	lps := []LockedProject{
		NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), nil),
		NewLockedProject(mkPI("foo"), NewVersion("nada"), nil),
		NewLockedProject(mkPI("bar"), NewVersion("zip"), nil),
		NewLockedProject(mkPI("qux"), NewVersion("zilch"), nil),
	}
	lps2 := make([]LockedProject, len(lps))
	copy(lps2, lps)

	SortLockedProjects(lps2)

	// only the two should have switched positions
	lps[0], lps[2] = lps[2], lps[0]
	if !reflect.DeepEqual(lps, lps2) {
		t.Errorf("SortLockedProject did not sort as expected:\n\t(GOT) %s\n\t(WNT) %s", lps2, lps)
	}
}

func TestLockedProjectsEq(t *testing.T) {
	lps := []LockedProject{
		NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), []string{"gps"}),
		NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), nil),
		NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), []string{"gps", "flugle"}),
		NewLockedProject(mkPI("foo"), NewVersion("nada"), []string{"foo"}),
		NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), []string{"flugle", "gps"}),
		NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0").Is("278a227dfc3d595a33a77ff3f841fd8ca1bc8cd0"), []string{"gps"}),
	}

	fix := []struct {
		l1, l2   int
		shouldeq bool
		err      string
	}{
		{0, 0, true, "lp does not eq self"},
		{0, 5, false, "should not eq with different rev"},
		{0, 1, false, "should not eq when other pkg list is empty"},
		{0, 2, false, "should not eq when other pkg list is longer"},
		{0, 4, false, "should not eq when pkg lists are out of order"},
		{0, 3, false, "should not eq totally different lp"},
	}

	for _, f := range fix {
		if f.shouldeq {
			if !lps[f.l1].Eq(lps[f.l2]) {
				t.Error(f.err)
			}
			if !lps[f.l2].Eq(lps[f.l1]) {
				t.Error(f.err + (" (reversed)"))
			}
		} else {
			if lps[f.l1].Eq(lps[f.l2]) {
				t.Error(f.err)
			}
			if lps[f.l2].Eq(lps[f.l1]) {
				t.Error(f.err + (" (reversed)"))
			}

		}
	}
}
