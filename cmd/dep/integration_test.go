// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/test"
	"github.com/golang/dep/internal/test/integration"
)

func TestIntegration(t *testing.T) {
	t.Parallel()

	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relPath := filepath.Join("testdata", "harness_tests")
	filepath.Walk(relPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatal("error walking filepath")
		}

		if filepath.Base(path) != "testcase.json" {
			return nil
		}

		parse := strings.Split(path, string(filepath.Separator))
		testName := strings.Join(parse[2:len(parse)-1], "/")
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			t.Run("external", testIntegration(testName, relPath, wd, execCmd))
			t.Run("internal", testIntegration(testName, relPath, wd, runMain))
		})

		return nil
	})
}

// execCmd is a test.RunFunc which runs the program in another process.
func execCmd(prog string, args []string, stdout, stderr io.Writer, dir string, env []string) error {
	cmd := exec.Command(prog, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	cmd.Dir = dir
	return cmd.Run()
}

// runMain is a test.RunFunc which runs the program in-process.
func runMain(prog string, args []string, stdout, stderr io.Writer, dir string, env []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch r := r.(type) {
			case error:
				err = r
			default:
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	m := &Config{
		Args:       append([]string{prog}, args...),
		Stdout:     stdout,
		Stderr:     stderr,
		WorkingDir: dir,
		Env:        env,
	}
	if exitCode := m.Run(); exitCode != 0 {
		err = fmt.Errorf("exit status %d", exitCode)
	}
	return
}

// testIntegration runs the test specified by <wd>/<relPath>/<name>/testcase.json
func testIntegration(name, relPath, wd string, run integration.RunFunc) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		// Set up environment
		testCase := integration.NewTestCase(t, filepath.Join(wd, relPath), name)
		testProj := integration.NewTestProject(t, testCase.InitialPath(), wd, run)
		defer testProj.Cleanup()

		// Create and checkout the vendor revisions
		for ip, rev := range testCase.VendorInitial {
			testProj.GetVendorGit(ip)
			testProj.RunGit(testProj.VendorPath(ip), "checkout", rev)
		}

		// Create and checkout the import revisions
		for ip, rev := range testCase.GopathInitial {
			testProj.RunGo("get", ip)
			testProj.RunGit(testProj.Path("src", ip), "checkout", rev)
		}

		// Run commands
		testProj.RecordImportPaths()

		var err error
		for i, args := range testCase.Commands {
			err = testProj.DoRun(args)
			if err != nil && i < len(testCase.Commands)-1 {
				t.Fatalf("cmd %s raised an unexpected error: %s", args[0], err.Error())
			}
		}

		// Check error raised in final command
		testCase.CompareError(err, testProj.GetStderr())

		if *test.UpdateGolden {
			testCase.UpdateOutput(testProj.GetStdout())
		} else {
			// Check output
			testCase.CompareOutput(testProj.GetStdout())
		}

		// Check vendor paths
		testProj.CompareImportPaths()
		testCase.CompareVendorPaths(testProj.GetVendorPaths())

		if *test.UpdateGolden {
			// Update manifest and lock
			testCase.UpdateFile(dep.ManifestName, testProj.ProjPath(dep.ManifestName))
			testCase.UpdateFile(dep.LockName, testProj.ProjPath(dep.LockName))
		} else {
			// Check final manifest and lock
			testCase.CompareFile(dep.ManifestName, testProj.ProjPath(dep.ManifestName))
			testCase.CompareFile(dep.LockName, testProj.ProjPath(dep.LockName))
		}
	}
}
