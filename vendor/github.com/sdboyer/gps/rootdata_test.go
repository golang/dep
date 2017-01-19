package gps

import (
	"reflect"
	"testing"
)

func TestRootdataExternalImports(t *testing.T) {
	fix := basicFixtures["shared dependency with overlapping constraints"]

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        fix.rootmanifest(),
	}

	is, err := Prepare(params, newdepspecSM(fix.ds, nil))
	if err != nil {
		t.Errorf("Unexpected error while prepping solver: %s", err)
		t.FailNow()
	}
	rd := is.(*solver).rd

	want := []string{"a", "b"}
	got := rd.externalImportList()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected return from rootdata.externalImportList:\n\t(GOT): %s\n\t(WNT): %s", got, want)
	}

	// Add a require
	rd.req["c"] = true

	want = []string{"a", "b", "c"}
	got = rd.externalImportList()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected return from rootdata.externalImportList:\n\t(GOT): %s\n\t(WNT): %s", got, want)
	}

	// Add same path as import
	poe := rd.rpt.Packages["root"]
	poe.P.Imports = []string{"a", "b", "c"}
	rd.rpt.Packages["root"] = poe

	// should still be the same
	got = rd.externalImportList()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected return from rootdata.externalImportList:\n\t(GOT): %s\n\t(WNT): %s", got, want)
	}

	// Add an ignore, but not on the required path (Prepare makes that
	// combination impossible)

	rd.ig["b"] = true
	want = []string{"a", "c"}
	got = rd.externalImportList()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected return from rootdata.externalImportList:\n\t(GOT): %s\n\t(WNT): %s", got, want)
	}
}

func TestGetApplicableConstraints(t *testing.T) {
	fix := basicFixtures["shared dependency with overlapping constraints"]

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        fix.rootmanifest(),
	}

	is, err := Prepare(params, newdepspecSM(fix.ds, nil))
	if err != nil {
		t.Errorf("Unexpected error while prepping solver: %s", err)
		t.FailNow()
	}
	rd := is.(*solver).rd

	table := []struct {
		name   string
		mut    func()
		result []workingConstraint
	}{
		{
			name: "base case, two constraints",
			mut:  func() {},
			result: []workingConstraint{
				{
					Ident:      mkPI("a"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("b"),
					Constraint: mkSVC("1.0.0"),
				},
			},
		},
		{
			name: "with unconstrained require",
			mut: func() {
				// No constraint means it doesn't show up
				rd.req["c"] = true
			},
			result: []workingConstraint{
				{
					Ident:      mkPI("a"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("b"),
					Constraint: mkSVC("1.0.0"),
				},
			},
		},
		{
			name: "with unconstrained import",
			mut: func() {
				// Again, no constraint means it doesn't show up
				poe := rd.rpt.Packages["root"]
				poe.P.Imports = []string{"a", "b", "d"}
				rd.rpt.Packages["root"] = poe
			},
			result: []workingConstraint{
				{
					Ident:      mkPI("a"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("b"),
					Constraint: mkSVC("1.0.0"),
				},
			},
		},
		{
			name: "constraint on required",
			mut: func() {
				rd.rm.Deps["c"] = ProjectProperties{
					Constraint: NewBranch("foo"),
				}
			},
			result: []workingConstraint{
				{
					Ident:      mkPI("a"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("b"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("c"),
					Constraint: NewBranch("foo"),
				},
			},
		},
		{
			name: "override on imported",
			mut: func() {
				rd.ovr["d"] = ProjectProperties{
					Constraint: NewBranch("bar"),
				}
			},
			result: []workingConstraint{
				{
					Ident:      mkPI("a"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("b"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("c"),
					Constraint: NewBranch("foo"),
				},
				{
					Ident:           mkPI("d"),
					Constraint:      NewBranch("bar"),
					overrConstraint: true,
				},
			},
		},
		{
			// It is certainly the simplest and most rule-abiding solution to
			// drop the constraint in this case, but is there a chance it would
			// violate the principle of least surprise?
			name: "ignore imported and overridden pkg",
			mut: func() {
				rd.ig["d"] = true
			},
			result: []workingConstraint{
				{
					Ident:      mkPI("a"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("b"),
					Constraint: mkSVC("1.0.0"),
				},
				{
					Ident:      mkPI("c"),
					Constraint: NewBranch("foo"),
				},
			},
		},
	}

	for _, fix := range table {
		fix.mut()

		got := rd.getApplicableConstraints()
		if !reflect.DeepEqual(fix.result, got) {
			t.Errorf("(fix: %q) unexpected applicable constraint set:\n\t(GOT): %+v\n\t(WNT): %+v", fix.name, got, fix.result)
		}
	}
}
