package main

import (
	"errors"
	"go/build"
	"testing"

	"github.com/golang/dep/internal/gps/pkgtree"
)

func TestCheckErrors(t *testing.T) {
	tt := []struct {
		name        string
		hasErrs     bool
		pkgOrErrMap map[string]pkgtree.PackageOrErr
	}{
		{
			name:    "noErrors",
			hasErrs: false,
			pkgOrErrMap: map[string]pkgtree.PackageOrErr{
				"mypkg": {
					P: pkgtree.Package{},
				},
			},
		},
		{
			name:    "hasErrors",
			hasErrs: true,
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
			if hasErrs := checkErrors(tc.pkgOrErrMap) != nil; hasErrs != tc.hasErrs {
				t.Fail()
			}
		})
	}
}
