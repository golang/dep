// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"path/filepath"

	"github.com/sdboyer/gps"
)

func (c *ctx) getSourceManager() (*gps.SourceMgr, error) {
	if c.GOPATH == "" {
		return nil, fmt.Errorf("GOPATH is not set")
	}
	// Use the first entry in GOPATH for the depcache
	first := filepath.SplitList(c.GOPATH)[0]

	return gps.NewSourceManager(analyzer{}, filepath.Join(first, "depcache"))
}
