// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"strings"
	"testing"
)

func assertEqual(t *testing.T, want, got interface{}, msg string) {
	if want != got {
		t.Fatalf("%s: want %q, got %q", msg, want, got)
	}
}

func TestYaml(t *testing.T) {
	rdr := strings.NewReader(`
# Testing

commands:
  - init
  - ensure -update
# Comment
  -remove    -unused

imports:
  github.com/sdboyer/deptest: 0.8.0
  # Comment
  github.com/sdboyer/deptestdos:     two words

`)
	doc, err := ParseYaml(rdr)
	if err != nil {
		t.Fatal(err)
	}

	wants := []string{"init", "ensure -update", "remove    -unused"}
	gots := doc["commands"].(YamlList)
	assertEqual(t, len(wants), len(gots), "doc.commands length")
	for ind, want := range wants {
		got := gots[ind]
		assertEqual(t, want, got, fmt.Sprintf("doc.commands[%d]", ind))
	}

	wants2 := map[string]string{"github.com/sdboyer/deptest": "0.8.0", "github.com/sdboyer/deptestdos": "two words"}
	gots2 := doc["imports"].(YamlDoc)
	for key, want := range wants2 {
		got := gots2[key]
		assertEqual(t, want, got, fmt.Sprintf("doc.imports[%s]", key))
	}

}

func TestYaml2(t *testing.T) {
	rdr := strings.NewReader(`
# ensure/update test case 4 - description here

commands:
  - init
  - ensure -update

imports:
    github.com/sdboyer/deptestdos: 1.0.0

initialVendors:
    github.com/sdboyer/deptest: 0.8.0

finalVendors:
  - github.com/sdboyer/deptest
  - github.com/sdboyer/deptestdos`)

	doc, err := ParseYaml(rdr)
	if err != nil {
		t.Fatal(err)
	}

	lwants := []string{"init", "ensure -update"}
	lgots := doc["commands"].(YamlList)
	assertEqual(t, len(lwants), len(lgots), "doc.commands length")
	for ind, want := range lwants {
		got := lgots[ind]
		assertEqual(t, want, got, fmt.Sprintf("doc.commands[%d]", ind))
	}

	mwants := map[string]string{
		"github.com/sdboyer/deptestdos": "1.0.0",
	}
	mgots := doc["imports"].(YamlDoc)
	for key, want := range mwants {
		got := mgots[key]
		assertEqual(t, want, got, fmt.Sprintf("doc.imports[%s]", key))
	}

	mwants = map[string]string{
		"github.com/sdboyer/deptest": "0.8.0",
	}
	mgots = doc["initialVendors"].(YamlDoc)
	for key, want := range mwants {
		got := mgots[key]
		assertEqual(t, want, got, fmt.Sprintf("doc.initialVendors[%s]", key))
	}

	lwants = []string{
		"github.com/sdboyer/deptest",
		"github.com/sdboyer/deptestdos",
	}
	lgots = doc["finalVendors"].(YamlList)
	assertEqual(t, len(lwants), len(lgots), "doc.finalVendors length")
	for ind, want := range lwants {
		got := lgots[ind]
		assertEqual(t, want, got, fmt.Sprintf("doc.finalVendors[%d]", ind))
	}
}

func TestYamlErrors(t *testing.T) {
	rdr := strings.NewReader(`
commands:
  - init
   - ensure -update
`)
	_, err := ParseYaml(rdr)
	if err == nil {
		t.Fatal("Should have returned error")
	}

	rdr = strings.NewReader(`
imports:
  github.com/sdboyer/deptestdos: 1.0.0
    github.com/sdboyer/deptestdos: 1.0.0
`)
	_, err = ParseYaml(rdr)
	if err == nil {
		t.Fatal("Should have returned error")
	}

	rdr = strings.NewReader(`
imports:
  key1:
    - bob
    key2:
      - fred
`)
	_, err = ParseYaml(rdr)
	if err == nil {
		t.Fatal("Should have returned error")
	}

}
