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
	}

	if !lps[0].Eq(lps[0]) {
		t.Error("lp does not eq self")
	}

	if lps[0].Eq(lps[1]) {
		t.Error("lp should not eq when other pkg list is empty")
	}
	if lps[1].Eq(lps[0]) {
		t.Fail()
	}

	if lps[0].Eq(lps[2]) {
		t.Error("lp should not eq when other pkg list is longer")
	}
	if lps[2].Eq(lps[0]) {
		t.Fail()
	}

	if lps[1].Eq(lps[2]) {
		t.Fail()
	}
	if lps[2].Eq(lps[1]) {
		t.Fail()
	}

	if lps[2].Eq(lps[4]) {
		t.Error("should not eq if pkgs are out of order")
	}

	if lps[0].Eq(lps[3]) {
		t.Error("lp should not eq totally different lp")
	}
}
