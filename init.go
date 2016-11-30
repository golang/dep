package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	`,
}

func runInit(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Too many args: %d", len(args))
	}
	var p string
	var err error
	if len(args) == 0 {
		p, err = os.Getwd()
		if err != nil {
			return err
		}
	} else {
		p = args[0]
	}

	m := filepath.Join(p, manifestName)

	// TODO: Lstat ? Do we care?
	_, err = os.Stat(m)
	if err != nil {
		if os.IsNotExist(err) {
			return createManifest(m)
		}
		return err
	}

	return fmt.Errorf("Manifest file '%s' already exists", m)
}

func createManifest(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	e := json.NewEncoder(f)
	return e.Encode(newRawManifest())
}
