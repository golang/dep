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
	"testing"
)

var (
	UpdateGolden *bool = flag.Bool("update", false, "update golden files")
)

// IntegrationTestCase manages a test case directory structure and content
type IntegrationTestCase struct {
	t             *testing.T
	Name          string
	RootPath      string
	InitialPath   string
	FinalPath     string
	ErrorExpected string            `json:"error-expected"`
	Commands      [][]string        `json:"commands"`
	GopathInitial map[string]string `json:"gopath-initial"`
	VendorInitial map[string]string `json:"vendor-initial"`
	VendorFinal   []string          `json:"vendor-final"`
}

func NewTestCase(t *testing.T, name string) *IntegrationTestCase {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	rootPath := filepath.FromSlash(filepath.Join(wd, "testdata", "harness_tests", name))
	n := &IntegrationTestCase{
		t:           t,
		Name:        name,
		RootPath:    rootPath,
		InitialPath: filepath.Join(rootPath, "initial"),
		FinalPath:   filepath.Join(rootPath, "final"),
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

// CompareError compares expected error to error recived.
func (tc *IntegrationTestCase) CompareError(err error) {
	wantExists, want := len(tc.ErrorExpected) > 0, tc.ErrorExpected
	gotExists, got := err != nil, ""
	if gotExists {
		got = err.Error()
	}

	if wantExists && gotExists {
		if want != got {
			tc.t.Errorf("expected error %s, got error %s", want, got)
		}
	} else if !wantExists && gotExists {
		tc.t.Fatalf("%s error raised where none was expected", got)
	} else if wantExists && !gotExists {
		tc.t.Errorf("%s error was not logged where one was expected", want)
	}
}

func (tc *IntegrationTestCase) CompareFile(goldenPath, working string) {
	golden := filepath.Join(tc.FinalPath, goldenPath)

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

func (tc *IntegrationTestCase) CompareVendorPaths(gotVendorPaths []string) {
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
