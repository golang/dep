// Copyright 2016-2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"reflect"
	"strings"
	"testing"

	"github.com/golang/dep/test"
	"github.com/sdboyer/gps"
)

func TestReadManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	mf := h.GetTestFile("manifest/golden.json")
	defer mf.Close()
	got, err := readManifest(mf)
	if err != nil {
		t.Fatalf("Should have read Manifest correctly, but got err %q", err)
	}

	c, _ := gps.NewSemverConstraint(">=0.12.0, <1.0.0")
	want := Manifest{
		Dependencies: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/sdboyer/gps"): {
				Constraint: c,
			},
			gps.ProjectRoot("github.com/babble/brook"): {
				Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
			},
		},
		Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/sdboyer/gps"): {
				Source:     "https://github.com/sdboyer/gps",
				Constraint: gps.NewBranch("master"),
			},
		},
		Ignores:  []string{"github.com/foo/bar"},
		Required: []string{"github.com/babble/brook"},
	}

	if !reflect.DeepEqual(got.Dependencies, want.Dependencies) {
		t.Error("Valid manifest's dependencies did not parse as expected")
	}
	if !reflect.DeepEqual(got.Ovr, want.Ovr) {
		t.Error("Valid manifest's overrides did not parse as expected")
	}
	if !reflect.DeepEqual(got.Ignores, want.Ignores) {
		t.Error("Valid manifest's ignores did not parse as expected")
	}
	if !reflect.DeepEqual(got.Required, want.Required) {
		t.Error("Valid manifest's requires did not parse as expected")
	}

	if !reflect.DeepEqual(got.Overrides(), want.Ovr) {
		t.Error("Manifest.Overrides() does not return expected overrides")
	}

	if gotLen, wantLen := len(got.IgnoredPackages()), len(want.Ignores); gotLen != wantLen {
		t.Errorf("Manifest.IgnoredPackages() returns %d elements, should be %d",
			gotLen, wantLen)
	}

	for k, v := range got.IgnoredPackages() {
		if v != true {
			t.Errorf("Manifest.IgnoredPackages() returns %v for %q, should be %v",
				v, k, true)
		}

		found := false
		for _, p := range want.Ignores {
			if k == p {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Manifest.IgnoredPackages() returns an unexpected element: %q", k)
		}
	}

	if gotLen, wantLen := len(got.RequiredPackages()), len(want.Required); gotLen != wantLen {
		t.Errorf("Manifest.RequiredPackages() returns %d elements, should be %d",
			gotLen, wantLen)
	}

	for k, v := range got.RequiredPackages() {
		if v != true {
			t.Errorf("Manifest.RequiredPackages() returns %v for %q, should be %v",
				v, k, true)
		}

		found := false
		for _, p := range want.Required {
			if k == p {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Manifest.RequiredPackages() returns an unexpected element: %q", k)
		}
	}
}

func TestWriteManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	golden := "manifest/golden.json"
	want := h.GetTestFileString(golden)
	c, _ := gps.NewSemverConstraint("^v0.12.0")
	m := &Manifest{
		Dependencies: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/sdboyer/gps"): {
				Constraint: c,
			},
			gps.ProjectRoot("github.com/babble/brook"): {
				Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
			},
		},
		Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/sdboyer/gps"): {
				Source:     "https://github.com/sdboyer/gps",
				Constraint: gps.NewBranch("master"),
			},
		},
		Ignores:  []string{"github.com/foo/bar"},
		Required: []string{"github.com/babble/brook"},
	}

	got, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("Error while marshaling valid manifest to JSON: %q", err)
	}

	if string(got) != want {
		if *test.UpdateGolden {
			if err = h.WriteTestFile(golden, string(got)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("Valid manifest did not marshal to JSON as expected:\n\t(GOT): %s\n\t(WNT): %s", string(got), want)
		}
	}
}

func TestReadManifestErrors(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	var err error

	tests := []struct {
		name string
		file string
	}{
		{"multiple constraints", "manifest/error.json"},
	}

	for _, tst := range tests {
		mf := h.GetTestFile(tst.file)
		defer mf.Close()
		_, err = readManifest(mf)
		if err == nil {
			t.Errorf("Reading manifest with %s should have caused error, but did not", tst.name)
		} else if !strings.Contains(err.Error(), tst.name) {
			t.Errorf("Unexpected error %q; expected %s error", err, tst.name)
		}
	}
}

func TestManifestInterface(t *testing.T) {
	c, _ := gps.NewSemverConstraint(">=0.12.0, <1.0.0")
	m := Manifest{
		Dependencies: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/sdboyer/gps"): {
				Constraint: c,
			},
			gps.ProjectRoot("github.com/babble/brook"): {
				Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
			},
		},
		Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/sdboyer/gps"): {
				Source:     "https://github.com/sdboyer/gps",
				Constraint: gps.NewBranch("master"),
			},
		},
		Ignores: []string{"github.com/foo/bar"},
	}

	var i gps.Manifest = &m

	if c := i.DependencyConstraints(); !reflect.DeepEqual(c, m.Dependencies) {
		t.Errorf("Manifest.DependencyConstraints() does not return expected constraints: %v vs %v",
			c, m.Dependencies)
	}

	// TODO decide whether we're going to incorporate this or not
	if c := i.TestDependencyConstraints(); c != nil {
		t.Errorf("Manifest.TestDependencyConstraints() does not return expected constraints: %v vs %v",
			c, nil)
	}
}
