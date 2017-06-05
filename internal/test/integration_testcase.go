// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

var (
	UpdateGolden *bool = flag.Bool("update", false, "update golden files")
)

// IntegrationTestCase manages a test case directory structure and content
type IntegrationTestCase struct {
	t             *testing.T
	name          string
	rootPath      string
	initialPath   string
	finalPath     string
	Commands      [][]string        `json:"commands"`
	ErrorExpected string            `json:"error-expected"`
	GopathInitial map[string]string `json:"gopath-initial"`
	VendorInitial map[string]string `json:"vendor-initial"`
	VendorFinal   []string          `json:"vendor-final"`
	InitPath      string            `json:"init-path"`
}

// NewTestCase creates a new IntegrationTestCase.
func NewTestCase(t *testing.T, dir, name string) *IntegrationTestCase {
	rootPath := filepath.FromSlash(filepath.Join(dir, name))
	n := &IntegrationTestCase{
		t:           t,
		name:        name,
		rootPath:    rootPath,
		initialPath: filepath.Join(rootPath, "initial"),
		finalPath:   filepath.Join(rootPath, "final"),
	}
	j, err := ioutil.ReadFile(filepath.Join(rootPath, "testcase.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(j, n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (tc *IntegrationTestCase) InitialPath() string {
	return tc.initialPath
}

// UpdateFile updates the golden file with the working result.
func (tc *IntegrationTestCase) UpdateFile(goldenPath, workingPath string) {
	exists, working, err := getFile(workingPath)
	if err != nil {
		tc.t.Fatalf("Error reading project file %s: %s", goldenPath, err)
	}

	golden := filepath.Join(tc.finalPath, goldenPath)
	if exists {
		if err := tc.WriteFile(golden, working); err != nil {
			tc.t.Fatal(err)
		}
	} else {
		err := os.Remove(golden)
		if err != nil && !os.IsNotExist(err) {
			tc.t.Fatal(err)
		}
	}
}

// CompareFile compares the golden file with the working result.
func (tc *IntegrationTestCase) CompareFile(goldenPath, working string) {
	golden := filepath.Join(tc.finalPath, goldenPath)

	gotExists, got, err := getFile(working)
	if err != nil {
		tc.t.Fatalf("Error reading project file %s: %s", goldenPath, err)
	}
	wantExists, want, err := getFile(golden)
	if err != nil {
		tc.t.Fatalf("Error reading testcase file %s: %s", goldenPath, err)
	}

	if wantExists && gotExists {
		if want != got {
			tc.t.Errorf("expected %s, got %s", want, got)
		}
	} else if !wantExists && gotExists {
		tc.t.Errorf("%s created where none was expected", goldenPath)
	} else if wantExists && !gotExists {
		tc.t.Errorf("%s not created where one was expected", goldenPath)
	}
}

// CompareError compares expected and actual stdout output
func (tc *IntegrationTestCase) CompareOutput(stdout string) {
	expected, err := ioutil.ReadFile(filepath.Join(tc.rootPath, "stdout.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to verify
			return
		}
		panic(err)
	}

	expStr := normalizeLines(string(expected))
	stdout = normalizeLines(stdout)

	if expStr != stdout {
		tc.t.Errorf("(WNT):\n%s\n(GOT):\n%s\n", expStr, stdout)
	}
}

// normalizeLines returns a version with trailing whitespace stripped from each line.
func normalizeLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

// CompareError compares exected and actual error
func (tc *IntegrationTestCase) CompareError(err error, stderr string) {
	wantExists, want := tc.ErrorExpected != "", tc.ErrorExpected
	gotExists, got := stderr != "" && err != nil, stderr

	if wantExists && gotExists {
		switch c := strings.Count(got, want); c {
		case 0:
			tc.t.Errorf("expected error containing %s, got error %s", want, got)
		case 1:
		default:
			tc.t.Errorf("expected error %s matches %d times to actual error %s", want, c, got)
		}
	} else if !wantExists && gotExists {
		tc.t.Fatalf("error raised where none was expected: \n%v", stderr)
	} else if wantExists && !gotExists {
		tc.t.Error("error not raised where one was expected:", want)
	}
}

func (tc *IntegrationTestCase) CompareVendorPaths(gotVendorPaths []string) {
	if *UpdateGolden {
		tc.VendorFinal = gotVendorPaths
	} else {
		wantVendorPaths := tc.VendorFinal
		if len(gotVendorPaths) != len(wantVendorPaths) {
			tc.t.Fatalf("Wrong number of vendor paths created: want %d got %d", len(wantVendorPaths), len(gotVendorPaths))
		}
		for ind := range gotVendorPaths {
			if gotVendorPaths[ind] != wantVendorPaths[ind] {
				tc.t.Errorf("Mismatch in vendor paths created: want %s got %s", wantVendorPaths, gotVendorPaths)
			}
		}
	}
}

func (tc *IntegrationTestCase) WriteFile(src string, content string) error {
	err := ioutil.WriteFile(src, []byte(content), 0666)
	return err
}

func getFile(path string) (bool, string, error) {
	_, err := os.Stat(path)
	if err != nil {
		return false, "", nil
	}
	f, err := ioutil.ReadFile(path)
	if err != nil {
		return true, "", err
	}
	return true, string(f), nil
}
