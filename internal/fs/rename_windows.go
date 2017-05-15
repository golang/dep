// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build windows

package fs

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
)

// RenameWithFallback attempts to rename a file or directory, but falls back to
// copying in the event of a cross-device link error. If the fallback copy
// succeeds, src is still removed, emulating normal rename behavior.
func RenameWithFallback(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "cannot stat %s", src)
	}

	if dstfi, err := os.Stat(dst); fi.IsDir() && err == nil && dstfi.IsDir() {
		return errors.Errorf("cannot rename directory %s to existing dst %s", src, dst)
	}

	err = os.Rename(src, dst)
	if err == nil {
		return nil
	}

	terr, ok := err.(*os.LinkError)
	if !ok {
		return err
	}

	// Rename may fail if src and dst are on different devices; fall back to
	// copy if we detect that case. syscall.EXDEV is the common name for the
	// cross device link error which has varying output text across different
	// operating systems.
	var cerr error
	if terr.Err != syscall.EXDEV {
		// In windows it can drop down to an operating system call that
		// returns an operating system error with a different number and
		// message. Checking for that as a fall back.
		noerr, ok := terr.Err.(syscall.Errno)

		// 0x11 (ERROR_NOT_SAME_DEVICE) is the windows error.
		// See https://msdn.microsoft.com/en-us/library/cc231199.aspx
		if ok && noerr != 0x11 {
			return errors.Wrapf(terr, "link error: cannot rename %s to %s", src, dst)
		}
	}

	if dir, _ := IsDir(src); dir {
		cerr = CopyDir(src, dst)
	} else {
		cerr = copyFile(src, dst)
	}

	if cerr != nil {
		return errors.Wrapf(cerr, "second attempt failed: cannot rename %s to %s", src, dst)
	}

	return errors.Wrapf(os.RemoveAll(src), "cannot delete %s", src)
}
