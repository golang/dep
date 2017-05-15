// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/golang/dep"
)

func TestGlideConvertProject(t *testing.T) {
	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}

	f := glideFiles{
		loggers: loggers,
		yaml: glideYaml{
			Imports: []glidePackage{
				{
					Name:       "github.com/sdboyer/deptest",
					Repository: "https://github.com/fork/deptest.git",
					Reference:  "v1.0.0",
				},
			},
		},
		lock: &glideLock{
			Imports: []glidePackage{
				{
					Name:      "github.com/sdboyer/deptest",
					Reference: "6a741be0cc55ecbe4f45690ebfd606a956d5f14a",
				},
			},
		},
	}

	manifest, lock, err := f.convert("", nil)
	if err != nil {
		t.Fatal(err)
	}

	d, ok := manifest.Dependencies["github.com/sdboyer/deptest"]
	if !ok {
		t.Fatal("Expected the manifest to have a dependency for 'github.com/sdboyer/deptest' but got none")
	}

	v := d.Constraint.String()
	if v != "v1.0.0" {
		t.Fatalf("Expected manifest constraint to be master, got %s", v)
	}

	if d.Source != "https://github.com/fork/deptest.git" {
		t.Fatalf("Expected manifest source to be 'https://github.com/fork/deptest.git', got %s", d.Source)
	}

	if len(lock.P) != 1 {
		t.Fatalf("Expected the lock to contain 1 project but got %d", len(lock.P))
	}

	p := lock.P[0]
	if p.Ident().ProjectRoot != "github.com/sdboyer/deptest" {
		t.Fatalf("Expected the lock to have a project for 'github.com/sdboyer/deptest' but got '%s'", p.Ident().ProjectRoot)
	}

	if p.Ident().Source != "https://github.com/fork/deptest.git" {
		t.Fatalf("Expected locked source to be 'https://github.com/fork/deptest.git', got '%s'", p.Ident().Source)
	}

	lv := p.Version().String()
	if lv != "6a741be0cc55ecbe4f45690ebfd606a956d5f14a" {
		t.Fatalf("Expected locked revision to be 'abc123', got %s", lv)
	}
}

func TestGlideConvertTestProject(t *testing.T) {
	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}

	f := glideFiles{
		loggers: loggers,
		yaml: glideYaml{
			TestImports: []glidePackage{
				{
					Name:      "github.com/sdboyer/deptest",
					Reference: "v1.0.0",
				},
			},
		},
		lock: &glideLock{
			TestImports: []glidePackage{
				{
					Name:      "github.com/sdboyer/deptest",
					Reference: "6a741be0cc55ecbe4f45690ebfd606a956d5f14a",
				},
			},
		},
	}

	manifest, lock, err := f.convert("", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := manifest.Dependencies["github.com/sdboyer/deptest"]
	if !ok {
		t.Fatal("Expected the manifest to have a dependency for 'github.com/sdboyer/deptest' but got none")
	}

	if len(lock.P) != 1 {
		t.Fatalf("Expected the lock to contain 1 project but got %d", len(lock.P))
	}
	p := lock.P[0]
	if p.Ident().ProjectRoot != "github.com/sdboyer/deptest" {
		t.Fatalf("Expected the lock to have a project for 'github.com/sdboyer/deptest' but got '%s'", p.Ident().ProjectRoot)
	}
}

func TestGlideConvertIgnore(t *testing.T) {
	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}

	f := glideFiles{
		loggers: loggers,
		yaml: glideYaml{
			Ignores: []string{"github.com/sdboyer/deptest"},
		},
	}

	manifest, _, err := f.convert("", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Ignored) != 1 {
		t.Fatalf("Expected the manifest to contain 1 ignored project but got %d", len(manifest.Ignored))
	}
	i := manifest.Ignored[0]
	if i != "github.com/sdboyer/deptest" {
		t.Fatalf("Expected the manifest to ignore 'github.com/sdboyer/deptest' but got '%s'", i)
	}
}

func TestGlideConvertExcludeDir(t *testing.T) {
	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}

	f := glideFiles{
		loggers: loggers,
		yaml: glideYaml{
			ExcludeDirs: []string{"samples"},
		},
	}

	manifest, _, err := f.convert("github.com/golang/notexist", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Ignored) != 1 {
		t.Fatalf("Expected the manifest to contain 1 ignored project but got %d", len(manifest.Ignored))
	}
	i := manifest.Ignored[0]
	if i != "github.com/golang/notexist/samples" {
		t.Fatalf("Expected the manifest to ignore 'github.com/golang/notexist/samples' but got '%s'", i)
	}
}

func TestGlideConvertExcludeDir_IgnoresMismatchedPackageName(t *testing.T) {
	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}

	f := glideFiles{
		loggers: loggers,
		yaml: glideYaml{
			Name:        "github.com/golang/mismatched-package-name",
			ExcludeDirs: []string{"samples"},
		},
	}

	manifest, _, err := f.convert("github.com/golang/notexist", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Ignored) != 1 {
		t.Fatalf("Expected the manifest to contain 1 ignored project but got %d", len(manifest.Ignored))
	}
	i := manifest.Ignored[0]
	if i != "github.com/golang/notexist/samples" {
		t.Fatalf("Expected the manifest to ignore 'github.com/golang/notexist/samples' but got '%s'", i)
	}
}

func TestGlideConvertWarnsForUnusedFields(t *testing.T) {
	testCases := map[string]glidePackage{
		"specified subpackages": {Subpackages: []string{"foo"}},
		"specified an os":       {OS: "windows"},
		"specified an arch":     {Arch: "i686"},
	}

	for wantWarning, pkg := range testCases {
		pkg.Name = "github.com/sdboyer/deptest"
		pkg.Reference = "v1.0.0"

		// Capture stderr so we can verify warnings
		verboseOutput := &bytes.Buffer{}
		loggers := &dep.Loggers{
			Out:     log.New(os.Stdout, "", 0),
			Err:     log.New(verboseOutput, "", 0),
			Verbose: true,
		}

		f := glideFiles{
			loggers: loggers,
			yaml: glideYaml{
				Imports: []glidePackage{pkg},
			},
		}

		_, _, err := f.convert("", nil)
		if err != nil {
			t.Fatal(err)
		}

		warnings := verboseOutput.String()
		if !strings.Contains(warnings, wantWarning) {
			t.Errorf("Expected the output to include the warning '%s'", wantWarning)
		}
	}
}

func TestGlideConvertBadInput_EmptyPackageName(t *testing.T) {
	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}

	f := glideFiles{
		loggers: loggers,
		yaml: glideYaml{
			Imports: []glidePackage{{Name: ""}},
		},
	}

	_, _, err := f.convert("", nil)
	if err == nil {
		t.Fatal("Expected conversion to fail because the package name is empty")
	}
}
