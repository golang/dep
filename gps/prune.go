// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// PruneOptions represents the pruning options used to write the dependecy tree.
type PruneOptions uint8

const (
	// PruneNestedVendorDirs indicates if nested vendor directories should be pruned.
	PruneNestedVendorDirs = 1 << iota
	// PruneUnusedPackages indicates if unused Go packages should be pruned.
	PruneUnusedPackages
	// PruneNonGoFiles indicates if non-Go files should be pruned.
	// Files matching licenseFilePrefixes and legalFileSubstrings are kept in
	// an attempt to comply with legal requirements.
	PruneNonGoFiles
	// PruneGoTestFiles indicates if Go test files should be pruned.
	PruneGoTestFiles
)

var (
	// licenseFilePrefixes is a list of name prefixes for license files.
	licenseFilePrefixes = []string{
		"license",
		"licence",
		"copying",
		"unlicense",
		"copyright",
		"copyleft",
	}
	// legalFileSubstrings contains substrings that are likey part of a legal
	// declaration file.
	legalFileSubstrings = []string{
		"authors",
		"contributors",
		"legal",
		"notice",
		"disclaimer",
		"patent",
		"third-party",
		"thirdparty",
	}
)

// Prune removes excess files from the dep tree whose root is baseDir based
// on the PruneOptions passed.
//
// A Lock must be passed if PruneUnusedPackages is toggled on.
func Prune(baseDir string, options PruneOptions, l Lock, logger *log.Logger) error {
	// TODO(ibrasho) allow passing specific options per project
	for _, lp := range l.Projects() {
		projectDir := filepath.Join(baseDir, string(lp.Ident().ProjectRoot))
		err := PruneProject(projectDir, lp, options, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

// PruneProject remove excess files according to the options passed, from
// the lp directory in baseDir.
func PruneProject(baseDir string, lp LockedProject, options PruneOptions, logger *log.Logger) error {
	projectDir := filepath.Join(baseDir, string(lp.Ident().ProjectRoot))

	if (options & PruneNestedVendorDirs) != 0 {
		if err := pruneNestedVendorDirs(projectDir); err != nil {
			return errors.Wrapf(err, "failed to prune nested vendor directories")
		}
	}

	if (options & PruneUnusedPackages) != 0 {
		if err := pruneUnusedPackages(lp, projectDir, logger); err != nil {
			return errors.Wrap(err, "failed to prune unused packages")
		}
	}

	if (options & PruneNonGoFiles) != 0 {
		if err := pruneNonGoFiles(projectDir, logger); err != nil {
			return errors.Wrap(err, "failed to prune non-Go files")
		}
	}

	if (options & PruneGoTestFiles) != 0 {
		if err := pruneGoTestFiles(projectDir, logger); err != nil {
			return errors.Wrap(err, "failed to prune Go test files")
		}
	}

	return nil
}

// pruneNestedVendorDirs deletes all nested vendor directories within baseDir.
func pruneNestedVendorDirs(baseDir string) error {
	return filepath.Walk(baseDir, stripVendor)
}

// pruneUnusedPackages deletes unimported packages found within baseDir.
// Determining whether packages are imported or not is based on the passed LockedProject.
func pruneUnusedPackages(lp LockedProject, projectDir string, logger *log.Logger) error {
	pr := string(lp.Ident().ProjectRoot)
	logger.Printf("Calculating unused packages in %s to prune.\n", pr)

	unusedPackages, err := calculateUnusedPackages(lp, projectDir)
	if err != nil {
		return errors.Wrapf(err, "could not calculate unused packages in %s", pr)
	}

	logger.Printf("Found the following unused packages in %s:\n", pr)
	for pkg := range unusedPackages {
		logger.Printf("  * %s\n", filepath.Join(pr, pkg))
	}

	unusedPackagesFiles, err := collectUnusedPackagesFiles(projectDir, unusedPackages)
	if err != nil {
		return errors.Wrapf(err, "could not collect unused packages' files in %s", pr)
	}

	if err := deleteFiles(unusedPackagesFiles); err != nil {
		return errors.Wrapf(err, "")
	}

	return nil
}

// calculateUnusedPackages generates a list of unused packages in lp.
func calculateUnusedPackages(lp LockedProject, projectDir string) (map[string]struct{}, error) {
	// TODO(ibrasho): optimize this...
	unused := make(map[string]struct{})
	imported := make(map[string]struct{})
	for _, pkg := range lp.Packages() {
		imported[pkg] = struct{}{}
	}

	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore anything that's not a directory.
		if !info.IsDir() {
			return nil
		}

		pkg, err := filepath.Rel(projectDir, path)
		if err != nil {
			return errors.Wrap(err, "unexpected error while calculating unused packages")
		}

		pkg = filepath.ToSlash(pkg)
		if _, ok := imported[pkg]; !ok {
			unused[pkg] = struct{}{}
		}

		return nil
	})

	return unused, err
}

// collectUnusedPackagesFiles returns a slice of all files in the unused packages in projectDir.
func collectUnusedPackagesFiles(projectDir string, unusedPackages map[string]struct{}) ([]string, error) {
	// TODO(ibrasho): is this useful?
	files := make([]string, 0, len(unusedPackages))

	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore directories.
		if info.IsDir() {
			return nil
		}

		// Ignore preserved files.
		if isPreservedFile(info.Name()) {
			return nil
		}

		pkg, err := filepath.Rel(projectDir, filepath.Dir(path))
		if err != nil {
			return errors.Wrap(err, "unexpected error while calculating unused packages")
		}

		pkg = filepath.ToSlash(pkg)
		if _, ok := unusedPackages[pkg]; ok {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// pruneNonGoFiles delete all non-Go files existing within baseDir.
// Files with names that are prefixed by any entry in preservedNonGoFiles
// are not deleted.
func pruneNonGoFiles(baseDir string, logger *log.Logger) error {
	files, err := collectNonGoFiles(baseDir, logger)
	if err != nil {
		return errors.Wrap(err, "could not collect non-Go files")
	}

	if err := deleteFiles(files); err != nil {
		return errors.Wrap(err, "could not prune Go test files")
	}

	return nil
}

// collectNonGoFiles returns a slice containing all non-Go files in baseDir.
// Files meeting the checks in isPreservedFile are not returned.
func collectNonGoFiles(baseDir string, logger *log.Logger) ([]string, error) {
	files := make([]string, 0)

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore directories.
		if info.IsDir() {
			return nil
		}

		// Ignore all Go files.
		if strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		// Ignore preserved files.
		if isPreservedFile(info.Name()) {
			return nil
		}

		files = append(files, path)

		return nil
	})

	return files, err
}

// isPreservedFile checks if the file name indicates that the file should be
// preserved based on licenseFilePrefixes or legalFileSubstrings.
func isPreservedFile(name string) bool {
	name = strings.ToLower(name)

	for _, prefix := range licenseFilePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	for _, substring := range legalFileSubstrings {
		if strings.Contains(name, substring) {
			return true
		}
	}

	return false
}

// pruneGoTestFiles deletes all Go test files (*_test.go) within baseDir.
func pruneGoTestFiles(baseDir string, logger *log.Logger) error {
	files, err := collectGoTestFiles(baseDir)
	if err != nil {
		return errors.Wrap(err, "could not collect Go test files")
	}

	if err := deleteFiles(files); err != nil {
		return errors.Wrap(err, "could not prune Go test files")
	}

	return nil
}

// collectGoTestFiles returns a slice contains all Go test files (any files
// prefixed with _test.go) in baseDir.
func collectGoTestFiles(baseDir string) ([]string, error) {
	files := make([]string, 0)

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore directories.
		if info.IsDir() {
			return nil
		}

		// Ignore any files that is not a Go test file.
		if strings.HasSuffix(info.Name(), "_test.go") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

func deleteFiles(paths []string) error {
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
