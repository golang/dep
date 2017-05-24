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
)

func TestIntegration(t *testing.T) {
	t.Parallel()

	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	filepath.Walk(filepath.Join("testdata", "harness_tests"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatal("error walking filepath")
		}

		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		if filepath.Base(path) == "testcase.json" {
			parse := strings.Split(path, string(filepath.Separator))
			testName := strings.Join(parse[2:len(parse)-1], "/")
			t.Run(testName, func(t *testing.T) {
				t.Parallel()

				t.Run("external", testIntegration(testName, wd, true, execCmd))
				t.Run("internal", testIntegration(testName, wd, false, runMain))
			})
		}
		return nil
	})

	filepath.Walk(filepath.Join("testdata", "init_path_tests"),
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				t.Fatal("error walking filepath")
			}

			wd, err := os.Getwd()
			if err != nil {
				panic(err)
			}

			if filepath.Base(path) == "testcase.json" {
				parse := strings.Split(path, string(filepath.Separator))
				testName := strings.Join(parse[2:len(parse)-1], "/")
				t.Run(testName, func(t *testing.T) {
					t.Parallel()

					t.Run("external", testRelativePath(testName, wd, true, execCmd))
					t.Run("internal", testRelativePath(testName, wd, false, runMain))
				})
			}
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

func testIntegration(name, wd string, externalProc bool, run test.RunFunc) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		// Set up environment
		testCase := test.NewTestCase(t, name, "harness_tests", wd)
		defer testCase.Cleanup()
		testProj := test.NewTestProject(t, testCase.InitialPath(), wd, externalProc, run)
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

		// Check output
		testCase.CompareOutput(testProj.GetStdout())

		// Check final manifest and lock
		testCase.CompareFile(dep.ManifestName, testProj.ProjPath(dep.ManifestName))
		testCase.CompareFile(dep.LockName, testProj.ProjPath(dep.LockName))

		// Check vendor paths
		testProj.CompareImportPaths()
		testCase.CompareVendorPaths(testProj.GetVendorPaths())
	}
}

func testRelativePath(name, wd string, externalProc bool, run test.RunFunc) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		// Set up environment
		testCase := test.NewTestCase(t, name, "init_path_tests", wd)
		defer testCase.Cleanup()
		testProj := test.NewTestProject(t, testCase.InitialPath(), wd, externalProc, run)
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

		// Check output
		testCase.CompareOutput(testProj.GetStdout())

		// Check final manifest and lock
		testCase.CompareFile(dep.ManifestName, testProj.ProjPath(dep.ManifestName))
		testCase.CompareFile(dep.LockName, testProj.ProjPath(dep.LockName))

		testProj.CompareImportPaths()
	}
}
