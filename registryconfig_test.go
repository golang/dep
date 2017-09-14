// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"reflect"
	"testing"
	"github.com/golang/dep/internal/test"
)

func TestReadConfig(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	mf := h.GetTestFile("registryconfig/golden.toml")
	defer mf.Close()
	got, err := readConfig(mf)
	if err != nil {
		t.Fatalf("Should have read Config correctly, but got err %q", err)
	}

	want := NewRegistryConfig("https://github.com/golang/dep", "2erygdasE45rty5JKwewrr75cb15rdeE")

	if !reflect.DeepEqual(got, want) {
		t.Error("Valid config did not parse as expected")
	}
}

func TestWriteConfig(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	golden := "registryconfig/golden.toml"
	want := h.GetTestFileString(golden)
	c := NewRegistryConfig("https://github.com/golang/dep", "2erygdasE45rty5JKwewrr75cb15rdeE")

	got, err := c.MarshalTOML()
	if err != nil {
		t.Fatalf("Error while marshaling valid manifest to TOML: %q", err)
	}

	if string(got) != want {
		if *test.UpdateGolden {
			if err = h.WriteTestFile(golden, string(got)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("Valid registry config did not marshal to TOML as expected:\n\t(GOT): %s\n\t(WNT): %s", string(got), want)
		}
	}
}
