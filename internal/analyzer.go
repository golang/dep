// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"os"
	"path/filepath"

	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/cfg"
	"github.com/golang/dep/internal/util"
)

type Analyzer struct{}

func (a Analyzer) DeriveManifestAndLock(path string, n gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	// TODO: If we decide to support other tools manifest, this is where we would need
	// to add that support.
	mf := filepath.Join(path, cfg.ManifestName)
	if fileOK, err := util.IsRegular(mf); err != nil || !fileOK {
		// Do not return an error, when does not exist.
		return nil, nil, nil
	}
	f, err := os.Open(mf)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	m, err := cfg.ReadManifest(f)
	if err != nil {
		return nil, nil, err
	}
	// TODO: No need to return lock til we decide about preferred versions, see
	// https://github.com/sdboyer/gps/wiki/gps-for-Implementors#preferred-versions.
	return m, nil, nil
}

func (a Analyzer) Info() (string, int) {
	return "dep", 1
}
