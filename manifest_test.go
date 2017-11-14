// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/test"
)

func TestReadManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	mf := h.GetTestFile("manifest/golden.toml")
	defer mf.Close()
	got, _, err := readManifest(mf)
	if err != nil {
		t.Fatalf("Should have read Manifest correctly, but got err %q", err)
	}

	c, _ := gps.NewSemverConstraint("^0.12.0")
	want := Manifest{
		Constraints: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/dep/gps"): {
				Constraint: c,
			},
			gps.ProjectRoot("github.com/babble/brook"): {
				Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
			},
		},
		Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/dep/gps"): {
				Source:     "https://github.com/golang/dep/gps",
				Constraint: gps.NewBranch("master"),
			},
		},
		Ignored: []string{"github.com/foo/bar"},
	}

	if !reflect.DeepEqual(got.Constraints, want.Constraints) {
		t.Error("Valid manifest's dependencies did not parse as expected")
	}
	if !reflect.DeepEqual(got.Ovr, want.Ovr) {
		t.Error("Valid manifest's overrides did not parse as expected")
	}
	if !reflect.DeepEqual(got.Ignored, want.Ignored) {
		t.Error("Valid manifest's ignored did not parse as expected")
	}
}

func TestWriteManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	golden := "manifest/golden.toml"
	want := h.GetTestFileString(golden)
	c, _ := gps.NewSemverConstraint("^0.12.0")
	m := NewManifest()
	m.Constraints[gps.ProjectRoot("github.com/golang/dep/gps")] = gps.ProjectProperties{
		Constraint: c,
	}
	m.Constraints[gps.ProjectRoot("github.com/babble/brook")] = gps.ProjectProperties{
		Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
	}
	m.Ovr[gps.ProjectRoot("github.com/golang/dep/gps")] = gps.ProjectProperties{
		Source:     "https://github.com/golang/dep/gps",
		Constraint: gps.NewBranch("master"),
	}
	m.Ignored = []string{"github.com/foo/bar"}

	got, err := m.MarshalTOML()
	if err != nil {
		t.Fatalf("Error while marshaling valid manifest to TOML: %q", err)
	}

	if string(got) != want {
		if *test.UpdateGolden {
			if err = h.WriteTestFile(golden, string(got)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("Valid manifest did not marshal to TOML as expected:\n\t(GOT): %s\n\t(WNT): %s", string(got), want)
		}
	}
}

func TestReadManifestErrors(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	var err error

	tests := []struct {
		name string
		file string
	}{
		{"multiple constraints", "manifest/error1.toml"},
		{"multiple dependencies", "manifest/error2.toml"},
		{"multiple overrides", "manifest/error3.toml"},
	}

	for _, tst := range tests {
		mf := h.GetTestFile(tst.file)
		defer mf.Close()
		_, _, err = readManifest(mf)
		if err == nil {
			t.Errorf("Reading manifest with %s should have caused error, but did not", tst.name)
		} else if !strings.Contains(err.Error(), tst.name) {
			t.Errorf("Unexpected error %q; expected %s error", err, tst.name)
		}
	}
}

func TestValidateManifest(t *testing.T) {
	cases := []struct {
		tomlString string
		wantWarn   []error
		wantError  error
	}{
		{
			tomlString: `
			required = ["github.com/foo/bar"]
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			required = "github.com/foo/bar"
			`,
			wantWarn:  []error{},
			wantError: errInvalidRequired,
		},
		{
			tomlString: `
			required = []
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			required = [1, 2, 3]
			`,
			wantWarn:  []error{},
			wantError: errInvalidRequired,
		},
		{
			tomlString: `
			[[required]]
			  name = "foo"
			`,
			wantWarn:  []error{},
			wantError: errInvalidRequired,
		},
		{
			tomlString: `
			ignored = ["foo"]
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			ignored = "foo"
			`,
			wantWarn:  []error{},
			wantError: errInvalidIgnored,
		},
		{
			tomlString: `
			ignored = []
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			ignored = [1, 2, 3]
			`,
			wantWarn:  []error{},
			wantError: errInvalidIgnored,
		},
		{
			tomlString: `
			[[ignored]]
			  name = "foo"
			`,
			wantWarn:  []error{},
			wantError: errInvalidIgnored,
		},
		{
			tomlString: `
			[metadata]
			  authors = "foo"
			  version = "1.0.0"
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			foo = "some-value"
			version = 14

			[[bar]]
			  author = "xyz"

			[[constraint]]
			  name = "github.com/foo/bar"
			  version = ""
			`,
			wantWarn: []error{
				errors.New("Unknown field in manifest: foo"),
				errors.New("Unknown field in manifest: bar"),
				errors.New("Unknown field in manifest: version"),
			},
			wantError: nil,
		},
		{
			tomlString: `
			metadata = "project-name"

			[[constraint]]
			  name = "github.com/foo/bar"
			`,
			wantWarn:  []error{errors.New("metadata should be a TOML table")},
			wantError: nil,
		},
		{
			tomlString: `
			[[constraint]]
			  name = "github.com/foo/bar"
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			[[constraint]]
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			constraint = "foo"
			`,
			wantWarn:  []error{},
			wantError: errInvalidConstraint,
		},
		{
			tomlString: `
			constraint = ["foo", "bar"]
			`,
			wantWarn:  []error{},
			wantError: errInvalidConstraint,
		},
		{
			tomlString: `
			[[override]]
			  name = "github.com/foo/bar"
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			[[override]]
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			override = "bar"
			`,
			wantWarn:  []error{},
			wantError: errInvalidOverride,
		},
		{
			tomlString: `
			override = ["foo", "bar"]
			`,
			wantWarn:  []error{},
			wantError: errInvalidOverride,
		},
		{
			tomlString: `
			[[constraint]]
			  name = "github.com/foo/bar"
			  location = "some-value"
			  link = "some-other-value"
			  metadata = "foo"

			[[override]]
			  nick = "foo"
			`,
			wantWarn: []error{
				errors.New("Invalid key \"location\" in \"constraint\""),
				errors.New("Invalid key \"link\" in \"constraint\""),
				errors.New("Invalid key \"nick\" in \"override\""),
				errors.New("metadata in \"constraint\" should be a TOML table"),
			},
			wantError: nil,
		},
		{
			tomlString: `
			[[constraint]]
			  name = "github.com/foo/bar"

			  [constraint.metadata]
			    color = "blue"
			`,
			wantWarn:  []error{},
			wantError: nil,
		},
		{
			tomlString: `
			[[constraint]]
			  name = "github.com/foo/bar"
			  revision = "b86ad16"
			`,
			wantWarn:  []error{errors.New("revision \"b86ad16\" should not be in abbreviated form")},
			wantError: nil,
		},
		{
			tomlString: `
			[[constraint]]
			  name = "foobar.com/hg"
			  revision = "8d43f8c0b836"
			`,
			wantWarn:  []error{errors.New("revision \"8d43f8c0b836\" should not be in abbreviated form")},
			wantError: nil,
		},
	}

	// contains for error
	contains := func(s []error, e error) bool {
		for _, a := range s {
			if a.Error() == e.Error() {
				return true
			}
		}
		return false
	}

	for _, c := range cases {
		errs, err := validateManifest(c.tomlString)

		// compare validation errors
		if err != c.wantError {
			t.Fatalf("Manifest errors are not as expected: \n\t(GOT) %v \n\t(WNT) %v", err, c.wantError)
		}

		// compare length of error slice
		if len(errs) != len(c.wantWarn) {
			t.Fatalf("Number of manifest errors are not as expected: \n\t(GOT) %v errors(%v)\n\t(WNT) %v errors(%v).", len(errs), errs, len(c.wantWarn), c.wantWarn)
		}

		// check if the expected errors exist in actual errors slice
		for _, er := range errs {
			if !contains(c.wantWarn, er) {
				t.Fatalf("Manifest errors are not as expected: \n\t(MISSING) %v\n\t(FROM) %v", er, c.wantWarn)
			}
		}
	}
}

func TestValidateProjectRoots(t *testing.T) {
	cases := []struct {
		name      string
		manifest  Manifest
		wantError error
		wantWarn  []string
	}{
		{
			name:      "empty Manifest",
			manifest:  Manifest{},
			wantError: nil,
			wantWarn:  []string{},
		},
		{
			name: "valid project root",
			manifest: Manifest{
				Constraints: map[gps.ProjectRoot]gps.ProjectProperties{
					gps.ProjectRoot("github.com/golang/dep"): {
						Constraint: gps.Any(),
					},
				},
			},
			wantError: nil,
			wantWarn:  []string{},
		},
		{
			name: "invalid project roots in Constraints and Overrides",
			manifest: Manifest{
				Constraints: map[gps.ProjectRoot]gps.ProjectProperties{
					gps.ProjectRoot("github.com/golang/dep/foo"): {
						Constraint: gps.Any(),
					},
					gps.ProjectRoot("github.com/golang/go/xyz"): {
						Constraint: gps.Any(),
					},
					gps.ProjectRoot("github.com/golang/fmt"): {
						Constraint: gps.Any(),
					},
				},
				Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
					gps.ProjectRoot("github.com/golang/mock/bar"): {
						Constraint: gps.Any(),
					},
					gps.ProjectRoot("github.com/golang/mock"): {
						Constraint: gps.Any(),
					},
				},
			},
			wantError: errInvalidProjectRoot,
			wantWarn: []string{
				"the name for \"github.com/golang/dep/foo\" should be changed to \"github.com/golang/dep\"",
				"the name for \"github.com/golang/mock/bar\" should be changed to \"github.com/golang/mock\"",
				"the name for \"github.com/golang/go/xyz\" should be changed to \"github.com/golang/go\"",
			},
		},
		{
			name: "invalid source path",
			manifest: Manifest{
				Constraints: map[gps.ProjectRoot]gps.ProjectProperties{
					gps.ProjectRoot("github.com/golang"): {
						Constraint: gps.Any(),
					},
				},
			},
			wantError: errInvalidProjectRoot,
			wantWarn:  []string{},
		},
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	pwd := h.Path(".")

	// Capture the stderr to verify the warnings
	stderrOutput := &bytes.Buffer{}
	errLogger := log.New(stderrOutput, "", 0)
	ctx := &Ctx{
		GOPATH: pwd,
		Out:    log.New(ioutil.Discard, "", 0),
		Err:    errLogger,
	}

	sm, err := ctx.SourceManager()
	h.Must(err)
	defer sm.Release()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Empty the buffer for every case
			stderrOutput.Reset()
			err := ValidateProjectRoots(ctx, &c.manifest, sm)
			if err != c.wantError {
				t.Fatalf("Unexpected error while validating project roots:\n\t(GOT): %v\n\t(WNT): %v", err, c.wantError)
			}

			warnings := stderrOutput.String()
			for _, warn := range c.wantWarn {
				if !strings.Contains(warnings, warn) {
					t.Fatalf("Expected ValidateProjectRoot errors to contain: %q", warn)
				}
			}
		})
	}
}
