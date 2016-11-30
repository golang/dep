package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

var initCmd = &command{
	fn:   runInit,
	name: "init",
	short: `
	Write Manifest file in the root of the project directory.
	`,
	long: `
	Populates Manifest file with current deps of this project.
	The specified version of each dependent repository is the version
	available in the user's workspaces (as specified by GOPATH).
	If the dependency is not present in any workspaces it is not be
	included in the Manifest.
	Writes Lock file(?)
	Creates vendor/ directory(?)

    Notes from DOC:
    Reads existing dependency information written by other tools.
    Noting any information that is lost (unsupported features, etc).
    This functionality will be removed after a transition period (1 year?).
    Write Manifest file in the root of the project directory.
    * Populates Manifest file with current deps of this project.
    The specified version of each dependent repository is the version available in the user's workspaces (including vendor/ directories, if present).
    If the dependency is not present in any workspaces it will not be included in the Manifest. A warning will be issued for these dependencies.
    Creates vendor/ directory (if it does not exist)
    Copies the project’s dependencies from the workspace to the vendor/ directory (if they’re not already there).
    Writes a Lockfile in the root of the project directory.
    Invoke “dep status”.
	`,
}

func determineProjectRoot(path string) (string, error) {
	gopath := os.Getenv("GOPATH")
	for _, gp := range filepath.SplitList(gopath) {
		srcprefix := filepath.Join(gp, "src") + string(filepath.Separator)
		if strings.HasPrefix(path, srcprefix) {
			// filepath.ToSlash because we're dealing with an import path now,
			// not an fs path
			return filepath.ToSlash(strings.TrimPrefix(path, srcprefix)), nil
		}
	}
	return "", fmt.Errorf("%s not in any $GOPATH", path)
}

// TODO: Error when there is a lockfile, but no manifest?
func runInit(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Too many args: %d", len(args))
	}
	var p string
	var err error
	if len(args) == 0 {
		p, err = os.Getwd()
		if err != nil {
			return errors.Wrap(err, "os.Getwd")
		}
	} else {
		p = args[0]
	}

	mf := filepath.Join(p, manifestName)

	// TODO: Lstat ? Do we care?
	_, err = os.Stat(mf)
	if err == nil {
		return fmt.Errorf("Manifest file '%s' already exists", mf)
	}
	if os.IsNotExist(err) {
		pr, err := determineProjectRoot(p)
		if err != nil {
			return errors.Wrap(err, "determineProjectRoot")
		}
		pkgT, err := gps.ListPackages(p, pr)
		if err != nil {
			return errors.Wrap(err, "gps.ListPackages")
		}
		sm, err := getSourceManager()
		if err != nil {
			return errors.Wrap(err, "getSourceManager")
		}
		defer sm.Release()
		m := newRawManifest()
		for _, v := range pkgT.Packages {
			// TODO: Some errors maybe should not be skipped ;-)
			if v.Err != nil {
				continue
			}

			for _, i := range v.P.Imports {
				if isStdLib(i) { // TODO: Replace with non stubbed version
					continue
				}
				pr, err := sm.DeduceProjectRoot(i)
				if err != nil {
					return errors.Wrap(err, "sm.DeduceProjectRoot") // TODO: Skip and report ?
				}
				// TODO: This is just wrong, need to figure out manifest file structure
				m.Dependencies[string(pr)] = possibleProps{}
			}
		}
		return errors.Wrap(writeManifest(mf, m), "writeManifest")
	}
	return errors.Wrap(err, "runInit fall through")
}

func isStdLib(i string) bool {
	switch i {
	case "bytes", "encoding/hex", "encoding/json", "flag", "fmt", "io", "os", "path/filepath", "strings", "text/tabwriter":
		return true
	}
	return false
}

func writeManifest(path string, m rawManifest) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	e := json.NewEncoder(f)
	return e.Encode(m)
}

func createManifest(path string) error {
	return writeManifest(path, newRawManifest())
}
