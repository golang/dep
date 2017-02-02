// Copyright 2016 The Go Authors. All rights reserved.
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

	_, err := readManifest(h.GetTestFileReader("manifest/error.json"))
	if err == nil {
		t.Error("Reading manifest with invalid props should have caused error, but did not")
	} else if !strings.Contains(err.Error(), "multiple constraints") {
		t.Errorf("Unexpected error %q; expected multiple constraint error", err)
	}

	m2, err := readManifest(h.GetTestFileReader("manifest/golden.json"))
	if err != nil {
		t.Fatalf("Should have read Manifest correctly, but got err %q", err)
	}

	c, _ := gps.NewSemverConstraint(">=0.12.0, <1.0.0")
	em := Manifest{
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

	if !reflect.DeepEqual(m2.Dependencies, em.Dependencies) {
		t.Error("Valid manifest's dependencies did not parse as expected")
	}
	if !reflect.DeepEqual(m2.Ovr, em.Ovr) {
		t.Error("Valid manifest's overrides did not parse as expected")
	}
	if !reflect.DeepEqual(m2.Ignores, em.Ignores) {
		t.Error("Valid manifest's ignores did not parse as expected")
	}
}

func TestWriteManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	jg := h.GetTestFileString("manifest/golden.json")
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
		Ignores: []string{"github.com/foo/bar"},
	}

	b, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("Error while marshaling valid manifest to JSON: %q", err)
	}

	if exp, err := test.AreEqualJSON(string(b), jg); !exp {
		h.Must(err)
		t.Errorf("Valid manifest did not marshal to JSON as expected:\n\t(GOT): %s\n\t(WNT): %s", string(b), jg)
	}
}
