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
	loggers *dep.Loggers
	sm      gps.SourceManager
}

func newGlideImporter(loggers *dep.Loggers, sm gps.SourceManager) glideImporter {
	return glideImporter{loggers: loggers, sm: sm}
}

func (i glideImporter) HasConfig(dir string) bool {
	// Only require glide.yaml, the lock is optional
	y := filepath.Join(dir, glideYamlName)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	return true
}

func (i glideImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	files := newGlideFiles(i.loggers)
	err := files.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return files.convert(string(pr), i.sm)
}
