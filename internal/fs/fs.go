// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
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
	// this function is more convoluted then ideal due to need for special
	// handling of volume name/drive letter on Windows. vnPath and vnPrefix
	// are first compared, and then used to initialize initial values of p and
	// d which will be appended to for incremental checks using
	// isCaseSensitiveFilesystem and then equality.

	// no need to check isCaseSensitiveFilesystem because VolumeName return
	// empty string on all non-Windows machines
	vnPath := strings.ToLower(filepath.VolumeName(path))
	vnPrefix := strings.ToLower(filepath.VolumeName(prefix))
	if vnPath != vnPrefix {
		return false
	}

	// because filepath.Join("c:","dir") returns "c:dir", we have to manually add path separator to drive letters
	if strings.HasSuffix(vnPath, ":") {
		vnPath += string(os.PathSeparator)
	}
	if strings.HasSuffix(vnPrefix, ":") {
		vnPrefix += string(os.PathSeparator)
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

	// [1:] in the lines below eliminates empty string on *nix and volume name on Windows
	dirs := strings.Split(dn, string(os.PathSeparator))[1:]
	prefixes := strings.Split(prefix, string(os.PathSeparator))[1:]

	if len(prefixes) > len(dirs) {
		return false
	}

	// d,p are initialized with "" on *nix and volume name on Windows
	d := vnPath
	p := vnPrefix

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

// RenameWithFallback attempts to rename a file or directory, but falls back to
// copying in the event of a cross-device link error. If the fallback copy
// succeeds, src is still removed, emulating normal rename behavior.
func RenameWithFallback(src, dst string) error {
	_, err := os.Stat(src)
	if err != nil {
		return errors.Wrapf(err, "cannot stat %s", src)
	}

	err = rename(src, dst)
	if err == nil {
		return nil
	}

	return renameFallback(err, src, dst)
}

// renameByCopy attempts to rename a file or directory by copying it to the
// destination and then removing the src thus emulating the rename behavior.
func renameByCopy(src, dst string) error {
	var cerr error
	if dir, _ := IsDir(src); dir {
		cerr = CopyDir(src, dst)
		if cerr != nil {
			cerr = errors.Wrap(cerr, "copying directory failed")
		}
	} else {
		cerr = copyFile(src, dst)
		if cerr != nil {
			cerr = errors.Wrap(cerr, "copying file failed")
		}
	}

	if cerr != nil {
		return errors.Wrapf(cerr, "rename fallback failed: cannot rename %s to %s", src, dst)
	}

	return errors.Wrapf(os.RemoveAll(src), "cannot delete %s", src)
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

var (
	errSrcNotDir = errors.New("source is not a directory")
	errDstExist  = errors.New("destination already exists")
)

// CopyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, destination directory must *not* exist.
func CopyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	// We use os.Lstat() here to ensure we don't fall in a loop where a symlink
	// actually links to a one of its parent directories.
	fi, err := os.Lstat(src)
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
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err = CopyDir(srcPath, dstPath); err != nil {
				return errors.Wrap(err, "copying directory failed")
			}
		} else {
			// This will include symlinks, which is what we want when
			// copying things.
			if err = copyFile(srcPath, dstPath); err != nil {
				return errors.Wrap(err, "copying file failed")
			}
		}
	}

	return nil
}

// copyFile copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all its contents will be replaced by the contents
// of the source file. The file mode will be copied from the source and
// the copied data is synced/flushed to stable storage.
func copyFile(src, dst string) (err error) {
	if sym, err := IsSymlink(src); err != nil {
		return err
	} else if sym {
		err := copySymlink(src, dst)
		return err
	}

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

// copySymlink will resolve the src symlink and create a new symlink in dst.
// If src is a relative symlink, dst will also be a relative symlink.
func copySymlink(src, dst string) error {
	resolved, err := os.Readlink(src)
	if err != nil {
		return errors.Wrap(err, "failed to resolve symlink")
	}

	err = os.Symlink(resolved, dst)
	if err != nil {
		return errors.Wrapf(err, "failed to create symlink %s to %s", src, resolved)
	}

	return nil
}

// IsDir determines is the path given is a directory or not.
func IsDir(name string) (bool, error) {
	// TODO: lstat?
	fi, err := os.Stat(name)
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
	if err != nil && !os.IsNotExist(err) {
		return false, err
	} else if !isDir {
		return false, nil
	}

	// Get file descriptor
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Query only 1 child. EOF if no children.
	_, err = f.Readdirnames(1)
	switch err {
	case io.EOF:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

// IsRegular determines if the path given is a regular file or not.
func IsRegular(name string) (bool, error) {
	fi, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	mode := fi.Mode()
	if mode&os.ModeType != 0 {
		return false, errors.Errorf("%q is a %v, expected a file", name, mode)
	}
	return true, nil
}

// IsSymlink determines if the given path is a symbolic link.
func IsSymlink(path string) (bool, error) {
	l, err := os.Lstat(path)
	if err != nil {
		return false, err
	}

	return l.Mode()&os.ModeSymlink == os.ModeSymlink, nil
}
