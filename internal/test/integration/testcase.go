// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/golang/dep/internal/test"
)

// TestCase manages a test case directory structure and content
type TestCase struct {
	t             *testing.T
	name          string
	rootPath      string
	initialPath   string
	finalPath     string
	Commands      [][]string        `json:"commands"`
	ShouldFail    bool              `json:"should-fail"`
	ErrorExpected string            `json:"error-expected"`
	GopathInitial map[string]string `json:"gopath-initial"`
	VendorInitial map[string]string `json:"vendor-initial"`
	VendorFinal   []string          `json:"vendor-final"`
	InitPath      string            `json:"init-path"`

	RequiredFeatureFlag string `json:"feature"`
}

// NewTestCase creates a new TestCase.
func NewTestCase(t *testing.T, dir, name string) *TestCase {
	rootPath := filepath.FromSlash(filepath.Join(dir, name))
	n := &TestCase{
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

	// Flip ShouldFail on if it's not set, but there's an expected error.
	if n.ErrorExpected != "" && !n.ShouldFail {
		n.ShouldFail = true
	}
	return n
}

// InitialPath represents the initial set of files in a project.
func (tc *TestCase) InitialPath() string {
	return tc.initialPath
}

// UpdateFile updates the golden file with the working result.
func (tc *TestCase) UpdateFile(goldenPath, workingPath string) {
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
func (tc *TestCase) CompareFile(goldenPath, working string) {
	golden := filepath.Join(tc.finalPath, goldenPath)

	gotExists, got, err := getFile(working)
	if err != nil {
		tc.t.Fatalf("Error reading project file %q: %s", goldenPath, err)
	}
	wantExists, want, err := getFile(golden)
	if err != nil {
		tc.t.Fatalf("Error reading testcase file %q: %s", goldenPath, err)
	}

	if wantExists && gotExists {
		if want != got {
			tc.t.Errorf("%s was not as expected\n(WNT):\n%s\n(GOT):\n%s", filepath.Base(goldenPath), want, got)
		}
	} else if !wantExists && gotExists {
		tc.t.Errorf("%q created where none was expected", goldenPath)
	} else if wantExists && !gotExists {
		tc.t.Errorf("%q not created where one was expected", goldenPath)
	}
}

// UpdateOutput updates the golden file for stdout with the working result.
func (tc *TestCase) UpdateOutput(stdout string) {
	stdoutPath := filepath.Join(tc.rootPath, "stdout.txt")
	_, err := os.Stat(stdoutPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Don't update the stdout.txt file if it doesn't exist.
			return
		}
		panic(err)
	}

	if err := tc.WriteFile(stdoutPath, stdout); err != nil {
		tc.t.Fatal(err)
	}
}

// CompareOutput compares expected and actual stdout output.
func (tc *TestCase) CompareOutput(stdout string) {
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
		tc.t.Errorf("stdout was not as expected\n(WNT):\n%s\n(GOT):\n%s\n", expStr, stdout)
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

// CompareError compares expected and actual stderr output.
func (tc *TestCase) CompareError(err error, stderr string) {
	wantExists, want := tc.ErrorExpected != "", tc.ErrorExpected
	gotExists, got := stderr != "" && err != nil, stderr

	if wantExists && gotExists {
		switch c := strings.Count(got, want); c {
		case 0:
			tc.t.Errorf("error did not contain expected string:\n\t(GOT): %s\n\t(WNT): %s", got, want)
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

// CompareCmdFailure checks to see if the failure/success (in the sense of an
// exit code) was as expected by the test fixture.
func (tc *TestCase) CompareCmdFailure(gotFail bool) {
	if gotFail == tc.ShouldFail {
		return
	}

	if tc.ShouldFail {
		tc.t.Errorf("expected command to fail, but it did not")
	} else {
		tc.t.Errorf("expected command not to fail, but it did")
	}
}

// CompareVendorPaths validates the vendor directory contents.
func (tc *TestCase) CompareVendorPaths(gotVendorPaths []string) {
	if *test.UpdateGolden {
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

// WriteFile writes a file using the default file permissions.
func (tc *TestCase) WriteFile(src string, content string) error {
	return ioutil.WriteFile(src, []byte(content), 0666)
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
