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

// To manage a test case directory structure and content
type IntegrationTestCase struct {
	t              *testing.T
	Name           string
	RootPath       string
	InitialPath    string
	FinalPath      string
	Commands       [][]string        `json:"commands"`
	Imports        map[string]string `json:"imports"`
	InitialVendors map[string]string `json:"initialVendors"`
	FinalVendors   []string          `json:"finalVendors"`
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

func (tc *IntegrationTestCase) CompareFile(goldenPath, working string) {
	golden := filepath.Join(tc.FinalPath, goldenPath)

	gotExists, got, err := getFile(working)
	if err != nil {
		panic(err)
	}
	wantExists, want, err := getFile(golden)
	if err != nil {
		panic(err)
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
	wantVendorPaths := tc.FinalVendors
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
