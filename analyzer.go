// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"

	"github.com/Masterminds/semver"
	"github.com/sdboyer/gps"
)

type analyzer struct{}

func (a analyzer) DeriveManifestAndLock(path string, n gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	// TODO: If we decide to support other tools manifest, this is where we would need
	// to add that support.
	mf := filepath.Join(path, ManifestName)
	if fileOK, err := IsRegular(mf); err != nil || !fileOK {
		// Do not return an error, when does not exist.
		return nil, nil, nil
	}
	f, err := os.Open(mf)
	if err != nil {
		return nil, nil, nil
	}
	defer f.Close()

	m, err := readManifest(f)
	if err != nil {
		return nil, nil, err
	}
	// TODO: No need to return lock til we decide about preferred versions, see
	// https://github.com/sdboyer/gps/wiki/gps-for-Implementors#preferred-versions.
	return m, nil, nil
}

func (a analyzer) Info() (name string, version *semver.Version) {
	v, _ := semver.NewVersion("v0.0.1")
	return "dep", v
}
