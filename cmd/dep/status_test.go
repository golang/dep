// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"testing"
	"text/tabwriter"

	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
)

func TestStatusFormatVersion(t *testing.T) {
	t.Parallel()

	tests := map[gps.Version]string{
		nil: "",
		gps.NewBranch("master"):        "branch master",
		gps.NewVersion("1.0.0"):        "1.0.0",
		gps.Revision("flooboofoobooo"): "flooboo",
	}
	for version, expected := range tests {
		str := formatVersion(version)
		if str != expected {
			t.Fatalf("expected '%v', got '%v'", expected, str)
		}
	}
}

func TestBasicLine(t *testing.T) {
	project := dep.Project{}
	aSemverConstraint, _ := gps.NewSemverConstraint("1.2.3")

	tests := []struct {
		name            string
		status          BasicStatus
		wantDotStatus   []string
		wantJSONStatus  []string
		wantTableStatus []string
	}{
		{
			name: "BasicStatus with ProjectRoot only",
			status: BasicStatus{
				ProjectRoot: "github.com/foo/bar",
			},
			wantDotStatus:   []string{`[label="github.com/foo/bar"];`},
			wantJSONStatus:  []string{`"Version":""`, `"Revision":""`},
			wantTableStatus: []string{`github.com/foo/bar                                         0`},
		},
		{
			name: "BasicStatus with Revision",
			status: BasicStatus{
				ProjectRoot: "github.com/foo/bar",
				Revision:    gps.Revision("flooboofoobooo"),
			},
			wantDotStatus:   []string{`[label="github.com/foo/bar\nflooboo"];`},
			wantJSONStatus:  []string{`"Version":""`, `"Revision":"flooboofoobooo"`, `"Constraint":""`},
			wantTableStatus: []string{`github.com/foo/bar                       flooboo           0`},
		},
		{
			name: "BasicStatus with Version and Revision",
			status: BasicStatus{
				ProjectRoot: "github.com/foo/bar",
				Version:     gps.NewVersion("1.0.0"),
				Revision:    gps.Revision("flooboofoobooo"),
			},
			wantDotStatus:   []string{`[label="github.com/foo/bar\n1.0.0"];`},
			wantJSONStatus:  []string{`"Version":"1.0.0"`, `"Revision":"flooboofoobooo"`, `"Constraint":""`},
			wantTableStatus: []string{`github.com/foo/bar              1.0.0    flooboo           0`},
		},
		{
			name: "BasicStatus with Constraint, Version and Revision",
			status: BasicStatus{
				ProjectRoot: "github.com/foo/bar",
				Constraint:  aSemverConstraint,
				Version:     gps.NewVersion("1.0.0"),
				Revision:    gps.Revision("revxyz"),
			},
			wantDotStatus:   []string{`[label="github.com/foo/bar\n1.0.0"];`},
			wantJSONStatus:  []string{`"Revision":"revxyz"`, `"Constraint":"1.2.3"`, `"Version":"1.0.0"`},
			wantTableStatus: []string{`github.com/foo/bar  1.2.3       1.0.0    revxyz            0`},
		},
		{
			name: "BasicStatus with update error",
			status: BasicStatus{
				ProjectRoot: "github.com/foo/bar",
				hasError:    true,
			},
			wantDotStatus:   []string{`[label="github.com/foo/bar"];`},
			wantJSONStatus:  []string{`"Version":""`, `"Revision":""`, `"Latest":"unknown"`},
			wantTableStatus: []string{`github.com/foo/bar                                 unknown  0`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer

			dotout := &dotOutput{
				p: &project,
				w: &buf,
			}
			dotout.BasicHeader()
			dotout.BasicLine(&test.status)
			dotout.BasicFooter()

			for _, wantStatus := range test.wantDotStatus {
				if ok := strings.Contains(buf.String(), wantStatus); !ok {
					t.Errorf("Did not find expected node status: \n\t(GOT) %v \n\t(WNT) %v", buf.String(), wantStatus)
				}
			}

			buf.Reset()

			jsonout := &jsonOutput{w: &buf}

			jsonout.BasicHeader()
			jsonout.BasicLine(&test.status)
			jsonout.BasicFooter()

			for _, wantStatus := range test.wantJSONStatus {
				if ok := strings.Contains(buf.String(), wantStatus); !ok {
					t.Errorf("Did not find expected JSON status: \n\t(GOT) %v \n\t(WNT) %v", buf.String(), wantStatus)
				}
			}

			buf.Reset()

			tabw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)

			tableout := &tableOutput{w: tabw}

			tableout.BasicHeader()
			tableout.BasicLine(&test.status)
			tableout.BasicFooter()

			for _, wantStatus := range test.wantTableStatus {
				if ok := strings.Contains(buf.String(), wantStatus); !ok {
					t.Errorf("Did not find expected Table status: \n\t(GOT) %v \n\t(WNT) %v", buf.String(), wantStatus)
				}
			}
		})
	}
}

func TestBasicStatusGetConsolidatedConstraint(t *testing.T) {
	aSemverConstraint, _ := gps.NewSemverConstraint("1.2.1")

	testCases := []struct {
		name           string
		basicStatus    BasicStatus
		wantConstraint string
	}{
		{
			name:           "empty BasicStatus",
			basicStatus:    BasicStatus{},
			wantConstraint: "",
		},
		{
			name: "BasicStatus with Any Constraint",
			basicStatus: BasicStatus{
				Constraint: gps.Any(),
			},
			wantConstraint: "*",
		},
		{
			name: "BasicStatus with Semver Constraint",
			basicStatus: BasicStatus{
				Constraint: aSemverConstraint,
			},
			wantConstraint: "1.2.1",
		},
		{
			name: "BasicStatus with Override",
			basicStatus: BasicStatus{
				Constraint:  aSemverConstraint,
				hasOverride: true,
			},
			wantConstraint: "1.2.1 (override)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.basicStatus.getConsolidatedConstraint() != tc.wantConstraint {
				t.Errorf("unexpected consolidated constraint: \n\t(GOT) %v \n\t(WNT) %v", tc.basicStatus.getConsolidatedConstraint(), tc.wantConstraint)
			}
		})
	}
}

func TestBasicStatusGetConsolidatedVersion(t *testing.T) {
	testCases := []struct {
		name        string
		basicStatus BasicStatus
		wantVersion string
	}{
		{
			name:        "empty BasicStatus",
			basicStatus: BasicStatus{},
			wantVersion: "",
		},
		{
			name: "BasicStatus with Version and Revision",
			basicStatus: BasicStatus{
				Version:  gps.NewVersion("1.0.0"),
				Revision: gps.Revision("revxyz"),
			},
			wantVersion: "1.0.0",
		},
		{
			name: "BasicStatus with only Revision",
			basicStatus: BasicStatus{
				Revision: gps.Revision("revxyz"),
			},
			wantVersion: "revxyz",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.basicStatus.getConsolidatedVersion() != tc.wantVersion {
				t.Errorf("unexpected consolidated version: \n\t(GOT) %v \n\t(WNT) %v", tc.basicStatus.getConsolidatedVersion(), tc.wantVersion)
			}
		})
	}
}

func TestBasicStatusGetConsolidatedLatest(t *testing.T) {
	testCases := []struct {
		name        string
		basicStatus BasicStatus
		revSize     uint8
		wantLatest  string
	}{
		{
			name:        "empty BasicStatus",
			basicStatus: BasicStatus{},
			revSize:     shortRev,
			wantLatest:  "",
		},
		{
			name: "nil latest",
			basicStatus: BasicStatus{
				Latest: nil,
			},
			revSize:    shortRev,
			wantLatest: "",
		},
		{
			name: "with error",
			basicStatus: BasicStatus{
				hasError: true,
			},
			revSize:    shortRev,
			wantLatest: "unknown",
		},
		{
			name: "short latest",
			basicStatus: BasicStatus{
				Latest: gps.Revision("adummylonglongrevision"),
			},
			revSize:    shortRev,
			wantLatest: "adummyl",
		},
		{
			name: "long latest",
			basicStatus: BasicStatus{
				Latest: gps.Revision("adummylonglongrevision"),
			},
			revSize:    longRev,
			wantLatest: "adummylonglongrevision",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotRev := tc.basicStatus.getConsolidatedLatest(tc.revSize)
			if gotRev != tc.wantLatest {
				t.Errorf("unexpected consolidated latest: \n\t(GOT) %v \n\t(WNT) %v", gotRev, tc.wantLatest)
			}
		})
	}
}
