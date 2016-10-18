// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sdboyer/gps"
)

func getSourceManager() (*gps.SourceMgr, error) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return nil, fmt.Errorf("GOPATH is not set")
	}
	// Use the first entry in GOPATH for the depcache
	first := filepath.SplitList(gopath)[0]

	return gps.NewSourceManager(analyzer{}, filepath.Join(first, "depcache"))
}
