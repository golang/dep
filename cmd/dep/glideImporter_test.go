// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
)

const testGlideProjectRoot string = "github.com/golang/notexist"

func TestGlideImport(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.TempDir(filepath.Join("src", testGlideProjectRoot))
	h.TempCopy(filepath.Join(testGlideProjectRoot, glideYamlName), "glide.yaml")
	h.TempCopy(filepath.Join(testGlideProjectRoot, glideLockName), "glide.lock")

	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}
	projectRoot := h.Path(testGlideProjectRoot)

	i := newGlideImporter(loggers)
	if !i.HasConfig(projectRoot) {
		t.Fatal("Expected the importer to detect the glide configuration files")
	}

	m, l, err := i.DeriveRootManifestAndLock(projectRoot, gps.ProjectRoot(testGlideProjectRoot))
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}
}

func TestGlideImport_MissingLockFile(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("src")
	h.TempDir(filepath.Join("src", "glidetest"))
	h.TempCopy(filepath.Join("glidetest", glideYamlName), "glide.yaml")

	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}
	projectRoot := h.Path("glidetest")

	i := newGlideImporter(loggers)
	if !i.HasConfig(projectRoot) {
		t.Fatal("The glide importer should gracefully handle when only glide.yaml is present")
	}

	m, l, err := i.DeriveRootManifestAndLock(projectRoot, gps.ProjectRoot(testGlideProjectRoot))
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l != nil {
		t.Fatal("Expected the lock to not be generated")
	}
}
