// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"go/build"
	"testing"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/gps/pkgtree"
)

func TestInvalidEnsureFlagCombinations(t *testing.T) {
	ec := &ensureCommand{
		update: true,
		add:    true,
	}

	if err := ec.validateFlags(); err == nil {
		t.Error("-add and -update together should fail validation")
	}

	ec.vendorOnly, ec.add = true, false
	if err := ec.validateFlags(); err == nil {
		t.Error("-vendor-only with -update should fail validation")
	}

	ec.add, ec.update = true, false
	if err := ec.validateFlags(); err == nil {
		t.Error("-vendor-only with -add should fail validation")
	}

	ec.noVendor, ec.add = true, false
	if err := ec.validateFlags(); err == nil {
		t.Error("-vendor-only with -no-vendor should fail validation")
	}
	ec.noVendor = false

	// Also verify that the plain ensure path takes no args. This is a shady
	// test, as lots of other things COULD return errors, and we don't check
	// anything other than the error being non-nil. For now, it works well
	// because a panic will quickly result if the initial arg length validation
	// checks are incorrectly handled.
	if err := ec.runDefault(nil, []string{"foo"}, nil, nil, gps.SolveParameters{}); err == nil {
		t.Errorf("no args to plain ensure with -vendor-only")
	}
	ec.vendorOnly = false
	if err := ec.runDefault(nil, []string{"foo"}, nil, nil, gps.SolveParameters{}); err == nil {
		t.Errorf("no args to plain ensure")
	}
}

func TestCheckErrors(t *testing.T) {
	tt := []struct {
		name        string
		fatal       bool
		pkgOrErrMap map[string]pkgtree.PackageOrErr
	}{
		{
			name:  "noErrors",
			fatal: false,
			pkgOrErrMap: map[string]pkgtree.PackageOrErr{
				"mypkg": {
					P: pkgtree.Package{},
				},
			},
		},
		{
			name:  "hasErrors",
			fatal: true,
			pkgOrErrMap: map[string]pkgtree.PackageOrErr{
				"github.com/me/pkg": {
					Err: &build.NoGoError{},
				},
				"github.com/someone/pkg": {
					Err: errors.New("code is busted"),
				},
			},
		},
		{
			name:  "onlyGoErrors",
			fatal: false,
			pkgOrErrMap: map[string]pkgtree.PackageOrErr{
				"github.com/me/pkg": {
					Err: &build.NoGoError{},
				},
				"github.com/someone/pkg": {
					P: pkgtree.Package{},
				},
			},
		},
		{
			name:  "onlyBuildErrors",
			fatal: false,
			pkgOrErrMap: map[string]pkgtree.PackageOrErr{
				"github.com/me/pkg": {
					Err: &build.NoGoError{},
				},
				"github.com/someone/pkg": {
					P: pkgtree.Package{},
				},
			},
		},
		{
			name:  "allGoErrors",
			fatal: true,
			pkgOrErrMap: map[string]pkgtree.PackageOrErr{
				"github.com/me/pkg": {
					Err: &build.NoGoError{},
				},
			},
		},
		{
			name:  "allMixedErrors",
			fatal: true,
			pkgOrErrMap: map[string]pkgtree.PackageOrErr{
				"github.com/me/pkg": {
					Err: &build.NoGoError{},
				},
				"github.com/someone/pkg": {
					Err: errors.New("code is busted"),
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			fatal, err := checkErrors(tc.pkgOrErrMap)
			if tc.fatal != fatal {
				t.Fatalf("expected fatal flag to be %T, got %T", tc.fatal, fatal)
			}
			if err == nil && fatal {
				t.Fatal("unexpected fatal flag value while err is nil")
			}
		})
	}
}
