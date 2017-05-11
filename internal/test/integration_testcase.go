// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
}

func NewTestCase(t *testing.T, name, wd string) *IntegrationTestCase {
	rootPath := filepath.FromSlash(filepath.Join(wd, "testdata", "harness_tests", name))
	n := &IntegrationTestCase{
		t:           t,
		name:        name,
		rootPath:    rootPath,
		initialPath: filepath.Join(rootPath, "initial"),
		finalPath:   filepath.Join(rootPath, "final"),
	}
	j, err := ioutil.ReadFile(filepath.Join(rootPath, "testcase.json"))
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(j, n)
	if err != nil {
		panic(err)
	}
	return n
}

var jsonNils *regexp.Regexp = regexp.MustCompile(`.*: null,.*\r?\n`)
var jsonCmds *regexp.Regexp = regexp.MustCompile(`(?s)  "commands": \[(.*)  ],`)
var jsonInds *regexp.Regexp = regexp.MustCompile(`(?s)\s*\n\s*`)

// Cleanup writes the resulting TestCase back to the directory, if the -update
// flag is set.  During the test, comparisons made to the TestCase should
// write the result back to the TestCase when -update is enabled
func (tc *IntegrationTestCase) Cleanup() {
	if *UpdateGolden {
		j, err := json.MarshalIndent(tc, "", "  ")
		if err != nil {
			panic(err)
		}
		j = jsonNils.ReplaceAll(j, []byte(""))
		cmds := jsonCmds.FindAllSubmatch(j, -1)[0][1]
		n := jsonInds.ReplaceAll(cmds, []byte(""))
		n = bytes.Replace(n, []byte("["), []byte("\n    ["), -1)
		n = bytes.Replace(n, []byte(`","`), []byte(`", "`), -1)
		n = append(n, '\n')
		j = bytes.Replace(j, cmds, n, -1)
		j = append(j, '\n')
		err = ioutil.WriteFile(filepath.Join(tc.rootPath, "testcase.json"), j, 0666)
		if err != nil {
			tc.t.Errorf("Failed to update testcase %s: %s", tc.name, err)
		}
	}
}

func (tc *IntegrationTestCase) InitialPath() string {
	return tc.initialPath
}

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
			if *UpdateGolden {
				if err := tc.WriteFile(golden, got); err != nil {
					tc.t.Fatal(err)
				}
			} else {
				tc.t.Errorf("expected %s, got %s", want, got)
			}
		}
	} else if !wantExists && gotExists {
		if *UpdateGolden {
			if err := tc.WriteFile(golden, got); err != nil {
				tc.t.Fatal(err)
			}
		} else {
			tc.t.Errorf("%s created where none was expected", goldenPath)
		}
	} else if wantExists && !gotExists {
		if *UpdateGolden {
			err := os.Remove(golden)
			if err != nil {
				tc.t.Fatal(err)
			}
		} else {
			tc.t.Errorf("%s not created where one was expected", goldenPath)
		}
	}
}

// CompareError compares exected and actual error
func (tc *IntegrationTestCase) CompareError(err error, stderr string) {
	wantExists, want := tc.ErrorExpected != "", tc.ErrorExpected
	gotExists, got := stderr != "" && err != nil, stderr

	if wantExists && gotExists {
		if !strings.Contains(got, want) {
			tc.t.Errorf("expected error containing %s, got error %s", want, got)
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
				tc.t.Errorf("Mismatch in vendor paths created: want %s got %s", gotVendorPaths, wantVendorPaths)
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
