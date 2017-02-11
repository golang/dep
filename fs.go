// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// IsRegular is true if name is a regular file
func IsRegular(name string) (bool, error) {
	// TODO: lstat?
	fi, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if fi.IsDir() {
		return false, fmt.Errorf("%q is a directory, should be a file", name)
	}
	return true, nil
}

// IsDir is true if name is a directory
func IsDir(name string) (bool, error) {
	// TODO: lstat?
	fi, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fi.IsDir() {
		return false, fmt.Errorf("%q is not a directory", name)
	}
	return true, nil
}

// IsEmptyDirOrNotExist is true if name is a directory and is not empty or
// doesn't exist.
// Returns an error when name is a file or on other fs/io errors.
func IsEmptyDirOrNotExist(name string) (bool, error) {
	files, err := ioutil.ReadDir(name)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return len(files) == 0, nil
}

func writeFile(path string, in json.Marshaler) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := in.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = f.Write(b)
	return err
}

// renameWithFallback attempts to rename a file or directory, but falls back to
// copying in the event of a cross-link device error. If the fallback copy
// succeeds, src is still removed, emulating normal rename behavior.
func renameWithFallback(src, dest string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// Windows cannot use syscall.Rename to rename a directory
	if runtime.GOOS == "windows" && fi.IsDir() {
		if err := CopyDir(src, dest); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}

	err = os.Rename(src, dest)
	if err == nil {
		return nil
	}

	terr, ok := err.(*os.LinkError)
	if !ok {
		return err
	}

	// Rename may fail if src and dest are on different devices; fall back to
	// copy if we detect that case. syscall.EXDEV is the common name for the
	// cross device link error which has varying output text across different
	// operating systems.
	var cerr error
	if terr.Err == syscall.EXDEV {
		if fi.IsDir() {
			cerr = CopyDir(src, dest)
		} else {
			cerr = CopyFile(src, dest)
		}
	} else if runtime.GOOS == "windows" {
		// In windows it can drop down to an operating system call that
		// returns an operating system error with a different number and
		// message. Checking for that as a fall back.
		noerr, ok := terr.Err.(syscall.Errno)
		// 0x11 (ERROR_NOT_SAME_DEVICE) is the windows error.
		// See https://msdn.microsoft.com/en-us/library/cc231199.aspx
		if ok && noerr == 0x11 {
			cerr = CopyFile(src, dest)
		}
	} else {
		return terr
	}

	if cerr != nil {
		return cerr
	}

	return os.RemoveAll(src)
}

// CopyDir takes in a directory and copies its contents to the destination.
// It preserves the file mode on files as well.
func CopyDir(src string, dest string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dest, fi.Mode())
	if err != nil {
		return err
	}

	dir, err := os.Open(src)
	if err != nil {
		return err
	}
	defer dir.Close()

	objects, err := dir.Readdir(-1)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		if obj.Mode()&os.ModeSymlink != 0 {
			continue
		}

		srcfile := filepath.Join(src, obj.Name())
		destfile := filepath.Join(dest, obj.Name())

		if obj.IsDir() {
			err = CopyDir(srcfile, destfile)
			if err != nil {
				return err
			}
			continue
		}

		if err := CopyFile(srcfile, destfile); err != nil {
			return err
		}
	}

	return nil
}

// CopyFile copies a file from one place to another with the permission bits
// preserved as well.
func CopyFile(src string, dest string) error {
	srcfile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcfile.Close()

	destfile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destfile.Close()

	if _, err := io.Copy(destfile, srcfile); err != nil {
		return err
	}

	srcinfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dest, srcinfo.Mode())
}
