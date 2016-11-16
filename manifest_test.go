// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sdboyer/gps"
)

func TestreadManifest(t *testing.T) {
	const je = `{
    "dependencies": {
        "github.com/sdboyer/gps": {
            "branch": "master",
            "revision": "d05d5aca9f895d19e9265839bffeadd74a2d2ecb",
            "version": "^v0.12.0",
            "network_name": "https://github.com/sdboyer/gps"
        }
    },
    "overrides": {
        "github.com/sdboyer/gps": {
            "branch": "master",
            "revision": "d05d5aca9f895d19e9265839bffeadd74a2d2ecb",
            "version": "^v0.12.0",
            "network_name": "https://github.com/sdboyer/gps"
        }
    },
    "ignores": [
        "github.com/foo/bar"
    ]
}`

	const jg = `{
    "dependencies": {
        "github.com/sdboyer/gps": {
            "version": "^v0.12.0"
        },
        "github.com/babble/brook": {
            "revision": "d05d5aca9f895d19e9265839bffeadd74a2d2ecb"
        }
    },
    "overrides": {
        "github.com/sdboyer/gps": {
            "branch": "master",
            "network_name": "https://github.com/sdboyer/gps"
        }
    },
    "ignores": [
        "github.com/foo/bar"
    ]
}`

	_, err := readManifest(strings.NewReader(je))
	if err == nil {
		t.Error("Reading manifest with invalid props should have caused error, but did not")
	} else if !strings.Contains(err.Error(), "multiple constraints") {
		t.Errorf("Unexpected error %q; expected multiple constraint error", err)
	}

	m2, err := readManifest(strings.NewReader(jg))
	if err != nil {
		t.Fatalf("Should have read Manifest correctly, but got err %q", err)
	}

	c, _ := gps.NewSemverConstraint("^v0.12.0")
	em := manifest{
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
				NetworkName: "https://github.com/sdboyer/gps",
				Constraint:  gps.NewBranch("master"),
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
