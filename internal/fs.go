// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
