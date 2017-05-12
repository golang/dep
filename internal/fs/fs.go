// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode"

	"github.com/pkg/errors"
)

// HasFilepathPrefix will determine if "path" starts with "prefix" from
// the point of view of a filesystem.
//
// Unlike filepath.HasPrefix, this function is path-aware, meaning that
// it knows that two directories /foo and /foobar are not the same
// thing, and therefore HasFilepathPrefix("/foobar", "/foo") will return
// false.
//
// This function also handles the case where the involved filesystems
// are case-insensitive, meaning /foo/bar and /Foo/Bar correspond to the
// same file. In that situation HasFilepathPrefix("/Foo/Bar", "/foo")
// will return true. The implementation is *not* OS-specific, so a FAT32
// filesystem mounted on Linux will be handled correctly.
func HasFilepathPrefix(path, prefix string) bool {
	if filepath.VolumeName(path) != filepath.VolumeName(prefix) {
		return false
	}

	var dn string

	if isDir, err := IsDir(path); err != nil {
		return false
	} else if isDir {
		dn = path
	} else {
		dn = filepath.Dir(path)
	}

	dn = strings.TrimSuffix(dn, string(os.PathSeparator))
	prefix = strings.TrimSuffix(prefix, string(os.PathSeparator))

	dirs := strings.Split(dn, string(os.PathSeparator))[1:]
	prefixes := strings.Split(prefix, string(os.PathSeparator))[1:]

	if len(prefixes) > len(dirs) {
		return false
	}

	var d, p string

	for i := range prefixes {
		// need to test each component of the path for
		// case-sensitiveness because on Unix we could have
		// something like ext4 filesystem mounted on FAT
		// mountpoint, mounted on ext4 filesystem, i.e. the
		// problematic filesystem is not the last one.
		if isCaseSensitiveFilesystem(filepath.Join(d, dirs[i])) {
			d = filepath.Join(d, dirs[i])
			p = filepath.Join(p, prefixes[i])
		} else {
			d = filepath.Join(d, strings.ToLower(dirs[i]))
			p = filepath.Join(p, strings.ToLower(prefixes[i]))
		}

		if p != d {
			return false
		}
	}

	return true
}

// isCaseSensitiveFilesystem determines if the filesystem where dir
// exists is case sensitive or not.
//
// CAVEAT: this function works by taking the last component of the given
// path and flipping the case of the first letter for which case
// flipping is a reversible operation (/foo/Bar → /foo/bar), then
// testing for the existence of the new filename. There are two
// possibilities:
//
// 1. The alternate filename does not exist. We can conclude that the
// filesystem is case sensitive.
//
// 2. The filename happens to exist. We have to test if the two files
// are the same file (case insensitive file system) or different ones
// (case sensitive filesystem).
//
// If the input directory is such that the last component is composed
// exclusively of case-less codepoints (e.g.  numbers), this function will
// return false.
func isCaseSensitiveFilesystem(dir string) bool {
	alt := filepath.Join(filepath.Dir(dir),
		genTestFilename(filepath.Base(dir)))

	dInfo, err := os.Stat(dir)
	if err != nil {
		return true
	}

	aInfo, err := os.Stat(alt)
	if err != nil {
		return true
	}

	return !os.SameFile(dInfo, aInfo)
}

// genTestFilename returns a string with at most one rune case-flipped.
//
// The transformation is applied only to the first rune that can be
// reversibly case-flipped, meaning:
//
// * A lowercase rune for which it's true that lower(upper(r)) == r
// * An uppercase rune for which it's true that upper(lower(r)) == r
//
// All the other runes are left intact.
func genTestFilename(str string) string {
	flip := true
	return strings.Map(func(r rune) rune {
		if flip {
			if unicode.IsLower(r) {
				u := unicode.ToUpper(r)
				if unicode.ToLower(u) == r {
					r = u
					flip = false
				}
			} else if unicode.IsUpper(r) {
				l := unicode.ToLower(r)
				if unicode.ToUpper(l) == r {
					r = l
					flip = false
				}
			}
		}
		return r
	}, str)
}

// RenameWithFallback attempts to rename a file or directory, but falls back to
// copying in the event of a cross-link device error. If the fallback copy
// succeeds, src is still removed, emulating normal rename behavior.
func RenameWithFallback(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "cannot stat %s", src)
	}

	if dstfi, err := os.Stat(src); err != nil {
		return errors.Wrapf(err, "cannot stat %s", dst)
	} else if dstfi.IsDir() {
		return errors.Errorf("dst %s is an existing directory", dst)
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
	if terr.Err == syscall.EXDEV {
		if fi.IsDir() {
			cerr = CopyDir(src, dst)
		} else {
			cerr = CopyFile(src, dst)
		}
	} else if runtime.GOOS == "windows" {
		// In windows it can drop down to an operating system call that
		// returns an operating system error with a different number and
		// message. Checking for that as a fall back.
		noerr, ok := terr.Err.(syscall.Errno)
		// 0x11 (ERROR_NOT_SAME_DEVICE) is the windows error.
		// See https://msdn.microsoft.com/en-us/library/cc231199.aspx
		if ok && noerr == 0x11 {
			if fi.IsDir() {
				cerr = CopyDir(src, dst)
			} else {
				cerr = CopyFile(src, dst)
			}
		}
	} else {
		return errors.Wrapf(terr, "link error: cannot rename %s to %s", src, dst)
	}

	if cerr != nil {
		return errors.Wrapf(cerr, "second attemp failed: cannot rename %s to %s", src, dst)
	}

	return errors.Wrapf(os.RemoveAll(src), "cannot delete %s", src)
}

var (
	errSrcNotDir = errors.New("source is not a directory")
	errDstExist  = errors.New("destination already exists")
)

// CopyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, destination directory must *not* exist.
// Symlinks are ignored and skipped.
func CopyDir(src string, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errSrcNotDir
	}

	_, err = os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		return errDstExist
	}

	if err = os.MkdirAll(dst, fi.Mode()); err != nil {
		return errors.Wrapf(err, "cannot mkdir %s", dst)
	}

	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return errors.Wrapf(err, "cannot read directory %s", dst)
	}

	for _, entry := range entries {
		if entry.Mode()&os.ModeSymlink != 0 {
			continue
		}

		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err = CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// This will include symlinks, which is what we want in all cases
			// where gps is copying things.
			if err = CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyFile copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file. The file mode will be copied from the source and
// the copied data is synced/flushed to stable storage.
func CopyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return
	}

	err = out.Sync()
	if err != nil {
		return
	}

	si, err := os.Stat(src)
	if err != nil {
		return
	}
	err = os.Chmod(dst, si.Mode())
	if err != nil {
		return
	}

	return
}

// IsDir determines is the path given is a directory or not.
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
		return false, errors.Errorf("%q is not a directory", name)
	}
	return true, nil
}

// IsNonEmptyDir determines if the path given is a non-empty directory or not.
func IsNonEmptyDir(name string) (bool, error) {
	isDir, err := IsDir(name)
	if !isDir || err != nil {
		return false, err
	}

	files, err := ioutil.ReadDir(name)
	if err != nil {
		return false, err
	}
	return len(files) != 0, nil
}

// IsRegular determines if the path given is a regular file or not.
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
		return false, errors.Errorf("%q is a directory, should be a file", name)
	}
	return true, nil
}
