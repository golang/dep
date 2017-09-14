// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"log"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/golang/dep/internal/test"
)

func TestCalculatePrune(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	vendorDir := "vendor"
	h.TempDir(vendorDir)
	h.TempDir(filepath.Join(vendorDir, "github.com/keep/pkg/sub"))
	h.TempDir(filepath.Join(vendorDir, "github.com/prune/pkg/sub"))

	toKeep := []string{
		filepath.FromSlash("github.com/keep/pkg"),
		filepath.FromSlash("github.com/keep/pkg/sub"),
	}

	discardLogger := log.New(ioutil.Discard, "", 0)

	got, err := calculatePrune(h.Path(vendorDir), toKeep, discardLogger)
	if err != nil {
		t.Fatal(err)
	}

	sort.Sort(byLen(got))

	want := []string{
		h.Path(filepath.Join(vendorDir, "github.com/prune/pkg/sub")),
		h.Path(filepath.Join(vendorDir, "github.com/prune/pkg")),
		h.Path(filepath.Join(vendorDir, "github.com/prune")),
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("calculated prune paths are not as expected.\n(WNT) %s\n(GOT) %s", want, got)
	}
}
