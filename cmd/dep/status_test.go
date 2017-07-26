// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"testing"

	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
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

	var tests = []struct {
		status   BasicStatus
		expected string
	}{
		{BasicStatus{
			Version:  nil,
			Revision: gps.Revision("flooboofoobooo"),
		}, `[label="\nflooboo"];`},
		{BasicStatus{
			Version:  gps.NewVersion("1.0.0"),
			Revision: gps.Revision("flooboofoobooo"),
		}, `[label="\n1.0.0"];`},
	}

	for _, test := range tests {
		var buf bytes.Buffer

		out := &dotOutput{
			p: &project,
			w: &buf,
		}
		out.BasicHeader()
		out.BasicLine(&test.status)
		out.BasicFooter()

		if ok := strings.Contains(buf.String(), test.expected); !ok {
			t.Fatalf("Did not find expected node label: \n\t(GOT) %v \n\t(WNT) %v", buf.String(), test.status)
		}
	}
}
