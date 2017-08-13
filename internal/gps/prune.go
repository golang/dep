// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/dep/internal/fs"
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
	// LICENSE & COPYING files are kept for convience.
	PruneNonGoFiles
	// PruneGoTestFiles indicates if Go test files should be pruned.
	PruneGoTestFiles
)

var (
	// licenseFilePrefixes is a list of name prefixes for licesnse files.
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
	if (options & PruneNestedVendorDirs) != 0 {
		if err := pruneNestedVendorDirs(baseDir); err != nil {
			return err
		}
	}

	if err := pruneEmptyDirs(baseDir, logger); err != nil {
		return errors.Wrap(err, "failed to prune empty dirs")
	}

	if (options & PruneUnusedPackages) != 0 {
		if l == nil {
			return errors.New("pruning unused packages requires passing a non-nil Lock")
		}
		if err := pruneUnusedPackages(baseDir, l, logger); err != nil {
			return errors.Wrap(err, "failed to prune unused packages")
		}
	}

	if (options & PruneNonGoFiles) != 0 {
		if err := pruneNonGoFiles(baseDir, logger); err != nil {
			return errors.Wrap(err, "failed to prune non-Go files")
		}
	}

	if (options & PruneGoTestFiles) != 0 {
		if err := pruneGoTestFiles(baseDir, logger); err != nil {
			return errors.Wrap(err, "failed to prune Go test files")
		}
	}

	// Delete all empty directories.
	if err := pruneEmptyDirs(baseDir, logger); err != nil {
		return errors.Wrap(err, "failed to prune empty dirs")
	}

	return nil
}

// pruneNestedVendorDirs deletes all nested vendor directories within baseDir.
func pruneNestedVendorDirs(baseDir string) error {
	return filepath.Walk(baseDir, stripNestedVendorDirs(baseDir))
}

// pruneUnusedPackages deletes unimported packages found within baseDir.
// Determining whether packages are imported or not is based on the passed Lock.
func pruneUnusedPackages(baseDir string, l Lock, logger *log.Logger) error {
	unused, err := calculateUnusedPackages(baseDir, l, logger)
	if err != nil {
		return err
	}

	for _, pkg := range unused {
		pkgPath := filepath.Join(baseDir, pkg)

		files, err := ioutil.ReadDir(pkgPath)
		if err != nil {
			// TODO(ibrasho) Handle this error properly.
			// It happens when attempting to ioutil.ReadDir a submodule.
			continue
		}

		// Delete *.go files in the package directory.
		for _, file := range files {
			// Skip directories and files that don't have a .go suffix.
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
				continue
			}

			if err := os.Remove(filepath.Join(pkgPath, file.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// calculateUnusedPackages generates a list of unused packages existing within
// baseDir depending on the imported packages found in the passed Lock.
func calculateUnusedPackages(baseDir string, l Lock, logger *log.Logger) ([]string, error) {
	imported := calculateImportedPackages(l)
	sort.Strings(imported)

	var unused []string

	if logger != nil {
		logger.Println("Calculating unused packages to prune.")
		logger.Println("Checking the following packages:")
	}

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore baseDir and anything that's not a directory.
		if path == baseDir || !info.IsDir() {
			return nil
		}

		pkg := strings.TrimPrefix(path, baseDir+string(filepath.Separator))
		if logger != nil {
			logger.Printf("  %s", pkg)
		}

		// If pkg is not a parent of an imported package, add it to the
		// unused list.
		i := sort.Search(len(imported), func(i int) bool {
			return pkg <= imported[i]
		})
		if i >= len(imported) || !strings.HasPrefix(imported[i], pkg) {
			unused = append(unused, path)
		}

		return nil
	})

	return unused, err
}

// calculateImportedPackages generates a list of imported packages from
// the passed Lock.
func calculateImportedPackages(l Lock) []string {
	var imported []string

	for _, project := range l.Projects() {
		projectRoot := string(project.Ident().ProjectRoot)
		for _, pkg := range project.Packages() {
			imported = append(imported, filepath.Join(projectRoot, pkg))
		}
	}
	return imported
}

// pruneNonGoFiles delete all non-Go files existing within baseDir.
// Files with names that are prefixed by any entry in preservedNonGoFiles
// are not deleted.
func pruneNonGoFiles(baseDir string, logger *log.Logger) error {
	files, err := calculateNonGoFiles(baseDir)
	if err != nil {
		return errors.Wrap(err, "could not prune non-Go files")
	}

	if err := deleteFiles(files); err != nil {
		return err
	}

	return nil
}

// calculateNonGoFiles returns a list of all non-Go files within baseDir.
// Files with names that are prefixed by any entry in preservedNonGoFiles
// are not deleted.
func calculateNonGoFiles(baseDir string) ([]string, error) {
	var files []string

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

		if !isPreservedNonGoFile(info.Name()) {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// isPreservedNonGoFile checks if the file name idicates that the file should be
// preserved. It assumes the file is not a Go file (doesn't have a .go suffix).
func isPreservedNonGoFile(name string) bool {
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

// pruneGoTestFiles deletes all Go test files (*_test.go) within baseDirr.
func pruneGoTestFiles(baseDir string, logger *log.Logger) error {
	files, err := calculateGoTestFiles(baseDir)
	if err != nil {
		return errors.Wrap(err, "could not prune Go test files")
	}

	if err := deleteFiles(files); err != nil {
		return err
	}

	return nil
}

// calculateGoTestFiles walks over baseDir and returns a list of all
// Go test files (any file that has the name *_test.go).
func calculateGoTestFiles(baseDir string) ([]string, error) {
	var files []string

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore directories.
		if info.IsDir() {
			return nil
		}

		// Ignore any files that is not a Go test file.
		if !strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		files = append(files, path)

		return nil
	})

	return files, err
}

// pruneEmptyDirs delete all empty directories within baseDir.
func pruneEmptyDirs(baseDir string, logger *log.Logger) error {
	empty, err := calculateEmptyDirs(baseDir)
	if err != nil {
		return err
	}

	if logger != nil {
		logger.Println("Deleting empty directories:")
	}

	for _, dir := range empty {
		if logger != nil {
			logger.Printf("  %s\n", strings.TrimPrefix(dir, baseDir+string(os.PathSeparator)))
		}
	}
	for _, dir := range empty {
		if err := os.Remove(dir); err != nil {
			return err
		}
	}

	return nil
}

// calculateEmptyDirs walks over baseDir and returns a slice of empty directory paths.
func calculateEmptyDirs(baseDir string) ([]string, error) {
	var empty []string

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if baseDir == path {
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		nonEmpty, err := fs.IsNonEmptyDir(path)
		if err != nil {
			return err
		} else if !nonEmpty {
			empty = append(empty, path)
		}

		return nil
	})

	return empty, err
}

func deleteFiles(paths []string) error {
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
