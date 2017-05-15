// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows
// +build !go1.8

package fs

import (
	"os"

	"github.com/pkg/errors"
)

func rename(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "cannot stat %s", src)
	}

	if dstfi, err := os.Stat(dst); fi.IsDir() && err == nil && dstfi.IsDir() {
		return errors.Errorf("cannot rename directory %s to existing dst %s", src, dst)
	}

	return os.Rename(src, dst)
}
