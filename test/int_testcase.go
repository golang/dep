// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	UpdateGolden *bool = flag.Bool("update", false, "update golden files")
)

// To manage a test case directory structure and content
type IntegrationTestCase struct {
	t           *testing.T
	Name        string
	RootPath    string
	InitialPath string
	FinalPath   string
	CommandPath string
	ImportPath  string
	VendorPath  string
}

func NewTestCase(t *testing.T, name string) *IntegrationTestCase {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	rootPath := filepath.Join(
		wd,
		"testdata",
		strings.Replace(name, "/", string(filepath.Separator), -1),
	)
	return &IntegrationTestCase{
		t:           t,
		Name:        name,
		RootPath:    rootPath,
		InitialPath: filepath.Join(rootPath, "initial"),
		FinalPath:   filepath.Join(rootPath, "final"),
		CommandPath: filepath.Join(rootPath, "commands.txt"),
		ImportPath:  filepath.Join(rootPath, "initial", "imports.txt"),
		VendorPath:  filepath.Join(rootPath, "final", "vendors.txt"),
	}
}

func (tc *IntegrationTestCase) GetImports() map[string]string {
	fpath := tc.ImportPath
	file, err := os.Open(fpath)
	if err != nil {
		panic(fmt.Sprintf("Opening %s produced error: %s", fpath, err))
	}

	result := make(map[string]string)
	content := bufio.NewReader(file)
	re := regexp.MustCompile(" +")
	lineNum := 1
	for err == nil {
		var line string
		line, err = content.ReadString('\n')
		line = strings.TrimSpace(line)
		if len(line) != 0 {
			parse := re.Split(line, -1)
			if len(parse) != 2 {
				panic(fmt.Sprintf("Malformed %s on line %d", fpath, lineNum))
			}
			result[parse[0]] = parse[1]
		}
		lineNum += 1
	}
	if err != io.EOF {
		panic(fmt.Sprintf("Reading %s produced error: %s", fpath, err))
	}
	return result
}

func (tc *IntegrationTestCase) GetCommands() [][]string {
	fpath := tc.CommandPath
	file, err := os.Open(fpath)
	if err != nil {
		panic(fmt.Sprintf("Opening %s produced error: %s", fpath, err))
	}

	result := make([][]string, 0)
	content := bufio.NewReader(file)
	re := regexp.MustCompile(" +")
	lineNum := 1
	for err == nil {
		var line string
		line, err = content.ReadString('\n')
		line = strings.TrimSpace(line)
		if len(line) != 0 {
			parse := re.Split(line, -1)
			if len(parse) < 1 {
				panic(fmt.Sprintf("Malformed %s on line %d", fpath, lineNum))
			}
			result = append(result, parse)
		}
		lineNum += 1
	}
	if err != io.EOF {
		panic(fmt.Sprintf("Reading %s produced error: %s", fpath, err))
	}
	return result
}

func (tc *IntegrationTestCase) GetVendors() []string {
	fpath := tc.VendorPath
	file, err := os.Open(fpath)
	if err != nil {
		panic(fmt.Sprintf("Opening %s produced error: %s", fpath, err))
	}

	result := make([]string, 0)
	content := bufio.NewReader(file)
	for err == nil {
		var line string
		line, err = content.ReadString('\n')
		line = strings.TrimSpace(line)
		if len(line) != 0 {
			result = append(result, line)
		}
	}
	if err != io.EOF {
		panic(fmt.Sprintf("Reading %s produced error: %s", fpath, err))
	}
	sort.Strings(result)
	return result
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
	if *UpdateGolden {
		content := strings.Join(gotVendorPaths, "\n") + "\n"
		if err := tc.WriteFile(tc.VendorPath, content); err != nil {
			tc.t.Fatal(err)
		}
	} else {
		wantVendorPaths := tc.GetVendors()
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
