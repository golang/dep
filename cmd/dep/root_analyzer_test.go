// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestRootAnalyzer_Info(t *testing.T) {
	testCases := map[bool]string{
		true:  "dep",
		false: "dep+import",
	}
	for skipTools, want := range testCases {
		a := rootAnalyzer{skipTools: skipTools}
		got, _ := a.Info()
		if got != want {
			t.Errorf("Expected the name of the importer with skipTools=%t to be '%s', got '%s'", skipTools, want, got)
		}
	}
}
