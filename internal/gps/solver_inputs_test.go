// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"testing"

	"github.com/golang/dep/internal/gps/pkgtree"
	"github.com/golang/dep/internal/test"
)

func TestValidateParams(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	cacheDir := "gps-cache"
	h.TempDir(cacheDir)
	sm, err := NewSourceManager(h.Path(cacheDir))
	h.Must(err)
	defer sm.Release()

	h.TempDir("src")

	testcases := []struct {
		imports []string
		err     bool
	}{
		{[]string{"google.com/non-existing/package"}, true},
		{[]string{"google.com/non-existing/package/subpkg"}, true},
		{[]string{"github.com/sdboyer/testrepo"}, false},
		{[]string{"github.com/sdboyer/testrepo/subpkg"}, false},
	}

	params := SolveParameters{
		ProjectAnalyzer: naiveAnalyzer{},
		RootDir:         h.Path("src"),
		RootPackageTree: pkgtree.PackageTree{
			ImportRoot: "github.com/sdboyer/dep",
		},
	}

	for _, tc := range testcases {
		params.RootPackageTree.Packages = map[string]pkgtree.PackageOrErr{
			"github.com/sdboyer/dep": {
				P: pkgtree.Package{
					Name:       "github.com/sdboyer/dep",
					ImportPath: "github.com/sdboyer/dep",
					Imports:    tc.imports,
				},
			},
		}

		err = ValidateParams(params, sm)
		if tc.err && err == nil {
			t.Fatalf("expected an error when deducing package fails, got none")
		} else if !tc.err && err != nil {
			t.Fatalf("deducing packges should have succeeded, got err: %#v", err)
		}
	}
}
