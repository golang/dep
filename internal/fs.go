// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode"

	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
)

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

func IsNonEmptyDir(name string) (bool, error) {
	isDir, err := IsDir(name)
	if !isDir || err != nil {
		return isDir, err
	}

	files, err := ioutil.ReadDir(name)
	if err != nil {
		return false, err
	}
	return len(files) != 0, nil
}

func writeFile(path string, in toml.Marshaler) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	s, err := in.MarshalTOML()
	if err != nil {
		return err
	}

	_, err = f.Write(s)
	return err
}

// modifyWithString modifies a given file with a new string input.
// This is used to write arbitrary string data to a file, such as
// updating the `Gopkg.toml` file with example data if no deps found
// on init.
func modifyWithString(path, data string) error {
	return ioutil.WriteFile(path, []byte(data), 0644)
}

// renameWithFallback attempts to rename a file or directory, but falls back to
// copying in the event of a cross-link device error. If the fallback copy
// succeeds, src is still removed, emulating normal rename behavior.
func renameWithFallback(src, dest string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return errors.Wrapf(err, "cannot stat %s", src)
	}

	// Windows cannot use syscall.Rename to rename a directory
	if runtime.GOOS == "windows" && fi.IsDir() {
		if err := CopyDir(src, dest); err != nil {
			return err
		}
		return errors.Wrapf(os.RemoveAll(src), "cannot delete %s", src)
	}

	err = os.Rename(src, dest)
	if err == nil {
		return nil
	}

	terr, ok := err.(*os.LinkError)
	if !ok {
		return errors.Wrapf(err, "cannot rename %s to %s", src, dest)
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
		return errors.Wrapf(terr, "link error: cannot rename %s to %s", src, dest)
	}

	if cerr != nil {
		return errors.Wrapf(cerr, "second attemp failed: cannot rename %s to %s", src, dest)
	}

	return errors.Wrapf(os.RemoveAll(src), "cannot delete %s", src)
}

// CopyDir takes in a directory and copies its contents to the destination.
// It preserves the file mode on files as well.
func CopyDir(src string, dest string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return errors.Wrapf(err, "cannot stat %s", src)
	}

	err = os.MkdirAll(dest, fi.Mode())
	if err != nil {
		return errors.Wrapf(err, "cannot mkdir %s", dest)
	}

	dir, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "cannot open %s", src)
	}
	defer dir.Close()

	objects, err := dir.Readdir(-1)
	if err != nil {
		return errors.Wrapf(err, "cannot read directory %s", dir.Name())
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
