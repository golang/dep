// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package importertest

import (
	"bytes"
	"io/ioutil"
	"log"
	"sort"
	"strings"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/test"
	"github.com/pkg/errors"
)

// TestCase is a common set of validations applied to the result
// of an importer converting from an external config format to dep's.
type TestCase struct {
	DefaultConstraintFromLock bool
	WantSourceRepo            string
	WantConstraint            string
	WantRevision              gps.Revision
	WantVersion               string
	WantIgnored               []string
	WantRequired              []string
	WantWarning               string
}

// NewTestContext creates a unique context with its own GOPATH for a single test.
func NewTestContext(h *test.Helper) *dep.Ctx {
	h.TempDir("src")
	pwd := h.Path(".")
	discardLogger := log.New(ioutil.Discard, "", 0)

	return &dep.Ctx{
		GOPATH: pwd,
		Out:    discardLogger,
		Err:    discardLogger,
	}
}

// Execute and validate the test case.
func (tc TestCase) Execute(t *testing.T, convert func(logger *log.Logger, sm gps.SourceManager) (*dep.Manifest, *dep.Lock)) error {
	h := test.NewHelper(t)
	defer h.Cleanup()
	// Disable parallel tests until we can resolve this error on the Windows builds:
	// "remote repository at https://github.com/carolynvs/deptest-importers does not exist, or is inaccessible"
	//h.Parallel()

	ctx := NewTestContext(h)
	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	// Capture stderr so we can verify warnings
	output := &bytes.Buffer{}
	ctx.Err = log.New(output, "", 0)

	manifest, lock := convert(ctx.Err, sm)
	return tc.validate(manifest, lock, output)
}

// validate returns an error if any of the testcase validations failed.
func (tc TestCase) validate(manifest *dep.Manifest, lock *dep.Lock, output *bytes.Buffer) error {
	if !equalSlice(manifest.Ignored, tc.WantIgnored) {
		return errors.Errorf("unexpected set of ignored projects: \n\t(GOT) %#v \n\t(WNT) %#v",
			manifest.Ignored, tc.WantIgnored)
	}

	if !equalSlice(manifest.Required, tc.WantRequired) {
		return errors.Errorf("unexpected set of required projects: \n\t(GOT) %#v \n\t(WNT) %#v",
			manifest.Required, tc.WantRequired)
	}

	wantConstraintCount := 0
	if tc.WantConstraint != "" {
		wantConstraintCount = 1
	}
	gotConstraintCount := len(manifest.Constraints)
	if gotConstraintCount != wantConstraintCount {
		return errors.Errorf("unexpected number of constraints: \n\t(GOT) %v \n\t(WNT) %v",
			gotConstraintCount, wantConstraintCount)
	}

	if tc.WantConstraint != "" {
		d, ok := manifest.Constraints[Project]
		if !ok {
			return errors.Errorf("Expected the manifest to have a dependency for '%v'",
				Project)
		}

		gotConstraint := d.Constraint.String()
		if gotConstraint != tc.WantConstraint {
			return errors.Errorf("unexpected constraint: \n\t(GOT) %v \n\t(WNT) %v",
				gotConstraint, tc.WantConstraint)
		}

	}

	// Lock checks.
	wantLockCount := 0
	if tc.WantRevision != "" {
		wantLockCount = 1
	}
	gotLockCount := 0
	if lock != nil {
		gotLockCount = len(lock.P)
	}
	if gotLockCount != wantLockCount {
		return errors.Errorf("unexpected number of locked projects: \n\t(GOT) %v \n\t(WNT) %v",
			gotLockCount, wantLockCount)
	}

	if tc.WantRevision != "" {
		lp := lock.P[0]

		gotProjectRoot := lp.Ident().ProjectRoot
		if gotProjectRoot != Project {
			return errors.Errorf("unexpected root project in lock: \n\t(GOT) %v \n\t(WNT) %v",
				gotProjectRoot, Project)
		}

		gotSource := lp.Ident().Source
		if gotSource != tc.WantSourceRepo {
			return errors.Errorf("unexpected source repository: \n\t(GOT) %v \n\t(WNT) %v",
				gotSource, tc.WantSourceRepo)
		}

		// Break down the locked "version" into a version (optional) and revision
		var gotVersion string
		var gotRevision gps.Revision
		if lpv, ok := lp.Version().(gps.PairedVersion); ok {
			gotVersion = lpv.String()
			gotRevision = lpv.Revision()
		} else if lr, ok := lp.Version().(gps.Revision); ok {
			gotRevision = lr
		} else {
			return errors.New("could not determine the type of the locked version")
		}

		if gotRevision != tc.WantRevision {
			return errors.Errorf("unexpected locked revision: \n\t(GOT) %v \n\t(WNT) %v",
				gotRevision,
				tc.WantRevision)
		}
		if gotVersion != tc.WantVersion {
			return errors.Errorf("unexpected locked version: \n\t(GOT) %v \n\t(WNT) %v",
				gotVersion,
				tc.WantVersion)
		}
	}

	if tc.WantWarning != "" {
		gotWarning := output.String()
		if !strings.Contains(gotWarning, tc.WantWarning) {
			return errors.Errorf("Expected the output to include the warning '%s' but got '%s'\n", tc.WantWarning, gotWarning)
		}
	}

	return nil
}

// equalSlice is comparing two string slices for equality.
func equalSlice(a, b []string) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
