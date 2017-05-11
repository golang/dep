// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
)

const glideYamlName = "glide.yaml"
const glideLockName = "glide.lock"

type glideImporter struct {
	loggers *Loggers
}

func (i glideImporter) Info() (name string, version int) {
	return "glide", 1
}

func (i glideImporter) HasConfig(dir string) bool {
	y := filepath.Join(dir, glideYamlName)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	l := filepath.Join(dir, glideLockName)
	if _, err := os.Stat(l); err != nil {
		return false
	}

	return true
}

func (i glideImporter) DeriveRootManifestAndLock(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	files := newGlideFiles(i.loggers)
	err := files.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return files.convert(string(pr))
}

func (i glideImporter) DeriveManifestAndLock(dir string, pr gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	return i.DeriveRootManifestAndLock(dir, pr)
}
