package gps

import (
	"reflect"
	"testing"
)

func TestLockedProjectSorting(t *testing.T) {
	// version doesn't matter here
	lps := []LockedProject{
		NewLockedProject("github.com/sdboyer/gps", NewVersion("v0.10.0"), "", nil),
		NewLockedProject("foo", NewVersion("nada"), "", nil),
		NewLockedProject("bar", NewVersion("zip"), "", nil),
		NewLockedProject("qux", NewVersion("zilch"), "", nil),
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
