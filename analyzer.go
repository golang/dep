// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/Masterminds/semver"
	"github.com/sdboyer/gps"
)

type analyzer struct{}

func (a analyzer) DeriveManifestAndLock(path string, n gps.ProjectRoot) (gps.Manifest, gps.Lock, error) {
	// TODO initial impl would just be looking for our own manifest and lock
	return nil, nil, nil
}

func (a analyzer) Info() (name string, version *semver.Version) {
	v, _ := semver.NewVersion("v0.0.1")
	return "example-analyzer", v
}
