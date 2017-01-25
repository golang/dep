// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/sdboyer/gps"
)

func TestStatusFormatVersion(t *testing.T) {

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
