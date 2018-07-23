// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/fs"
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
			// When updating test case files we can't run in parallel, because then
			// we would have a race condition (each test case is used twice, once
			// for "internal" and again for "external").
			if !*test.UpdateGolden {
				t.Parallel()
			}

			t.Run("external", testIntegration(testName, relPath, wd, execCmd))
			t.Run("internal", testIntegration(testName, relPath, wd, runMain))
		})

		return nil
	})
}

func TestDepCachedir(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This test is unreliable on Windows and fails at random which makes it very
		// difficult to debug. It might have something to do with parallel execution.
		// Since the test doesn't test any specific behavior of Windows, it should be okay
		// to skip.
		t.Skip("skipping on windows")
	}
	t.Parallel()

	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	initPath := filepath.Join("testdata", "cachedir")

	t.Run("env-invalid-cachedir", func(t *testing.T) {
		t.Parallel()
		testProj := integration.NewTestProject(t, initPath, wd, runMain)
		defer testProj.Cleanup()

		var d []byte
		tmpFp := testProj.Path("tmp-file")
		ioutil.WriteFile(tmpFp, d, 0644)
		cases := []string{
			// invalid path
			"\000",
			// parent directory does not exist
			testProj.Path("non-existent-fldr", "cachedir"),
			// path is a regular file
			tmpFp,
			// invalid path, tmp-file is a regular file
			testProj.Path("tmp-file", "cachedir"),
		}

		wantErr := "dep: $DEPCACHEDIR set to an invalid or inaccessible path"
		for _, c := range cases {
			testProj.Setenv("DEPCACHEDIR", c)

			err = testProj.DoRun([]string{"ensure"})

			if err == nil {
				// Log the output from running `dep ensure`, could be useful.
				t.Logf("test run output: \n%s\n%s", testProj.GetStdout(), testProj.GetStderr())
				t.Error("unexpected result: \n\t(GOT) nil\n\t(WNT) exit status 1")
			} else if stderr := testProj.GetStderr(); !strings.Contains(stderr, wantErr) {
				t.Errorf(
					"unexpected error output: \n\t(GOT) %s\n\t(WNT) %s",
					strings.TrimSpace(stderr), wantErr,
				)
			}
		}
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

		testCase := integration.NewTestCase(t, filepath.Join(wd, relPath), name)

		// Skip tests for disabled features
		if testCase.RequiredFeatureFlag != "" {
			featureEnabled, err := readFeatureFlag(testCase.RequiredFeatureFlag)
			if err != nil {
				t.Fatal(err)
			}

			if !featureEnabled {
				t.Skipf("skipping %s, %s feature flag not enabled", name, testCase.RequiredFeatureFlag)
			}
		}

		// Set up environment
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

		if err != nil {
			t.Log(err)
		}

		// Check error raised in final command
		testCase.CompareCmdFailure(err != nil)
		testCase.CompareError(err, testProj.GetStderr())

		if *test.UpdateGolden {
			testCase.UpdateOutput(testProj.GetStdout())
		} else {
			// Check output
			testCase.CompareOutput(testProj.GetStdout())
		}

		// Determine how the test case specifies the expected
		// content: either it lists just some projects, or it
		// provides a complete reference directory.
		reference := len(testCase.VendorFinal) == 1 && testCase.VendorFinal[0] == "compare"

		// Check vendor paths
		testProj.CompareImportPaths()
		if !reference {
			testCase.CompareVendorPaths(testProj.GetVendorPaths())
		}

		if reference {
			// Check all files.
			if *test.UpdateGolden {
				// Update all files in the 'final' directory, removing those which
				// no longer should exist.
				if err := os.RemoveAll(testCase.FinalPath()); err != nil {
					t.Fatalf("error removing 'final' directory: %s", err)
				}
				if err := fs.CopyDir(testProj.ProjPath(), testCase.FinalPath()); err != nil {
					t.Fatalf("error copying into 'final' directory: %s", err)
				}
			} else {
				// Compare all files from either of the two trees.
				files := make(map[string]bool)
				findFiles := func(dir string) error {
					return filepath.Walk(dir,
						func(path string, info os.FileInfo, err error) error {
							if err != nil {
								return err
							}
							if dir == path {
								return nil
							}
							if !info.IsDir() {
								localpath := path[len(dir)+1:]
								files[localpath] = true
							}
							return nil
						})
				}
				findFiles(testCase.FinalPath())
				findFiles(testProj.ProjPath())

				t.Logf("checking against %s", testCase.FinalPath())
				for localpath := range files {
					t.Logf("comparing %s", localpath)
					testCase.CompareFile(localpath, testProj.ProjPath(localpath))
				}
			}
		} else {
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
}
