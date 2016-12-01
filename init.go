package main

import (
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

		// TODO: This is just wrong, need to figure out manifest file structure
		m := manifest{
			Dependencies: make(gps.ProjectConstraints),
		}

		processed := make(map[gps.ProjectRoot]bool)
		ondisk := make(map[gps.ProjectRoot]gps.Version)
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

				if processed[pr] {
					continue
				}
				processed[pr] = true

				v, err := whatVersionIsOnDiskForThisThing(pr)
				if err != nil {
					fmt.Printf("Could not determine version for %q, omitting from generated manifest\n", pr)
					continue
				}

				ondisk[pr] = v
				pp := gps.ProjectProperties{}
				switch v.Type() {
				case "branch", "version", "rev":
					pp.Constraint = v
				case "semver":
					c, _ := gps.NewSemverConstraint("^" + v.String())
					pp.Constraint = c
				}

				m.Dependencies[pr] = pp
			}
		}

		return errors.Wrap(writeManifest(mf, &m), "writeManifest")
	}
	return errors.Wrap(err, "runInit fall through")
}

// TODO this is a stub, make it not a stub when gps gets its act together
func isStdLib(i string) bool {
	switch i {
	case "bytes", "encoding/hex", "errors", "sort", "encoding/json", "flag", "fmt", "io", "os", "path/filepath", "strings", "text/tabwriter":
		return true
	}
	return false
}

// TODO rename this to something not dumb when it becomes not a stub
func whatVersionIsOnDiskForThisThing(pr gps.ProjectRoot) (gps.Version, error) {
	switch pr {
	case "github.com/sdboyer/gps":
		return gps.NewVersion("v0.12.0").Is("9ca61cb4e9851c80bb537e7d8e1be56e18e03cc9"), nil
	case "github.com/Masterminds/semver":
		return gps.NewBranch("2.x").Is("b3ef6b1808e9889dfb8767ce7068db923a3d07de"), nil
	case "github.com/pkg/errors":
		return gps.NewVersion("v0.8.0").Is("645ef00459ed84a119197bfb8d8205042c6df63d"), nil
	}

	return nil, fmt.Errorf("unknown project")
}

func writeManifest(path string, m *manifest) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := m.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = f.Write(b)
	return err
}
