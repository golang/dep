package vsolver

import (
	"bytes"
	"go/build"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/scanner"
)

var osList []string
var archList []string

func init() {
	// The supported systems are listed in
	// https://github.com/golang/go/blob/master/src/go/build/syslist.go
	// The lists are not exported so we need to duplicate them here.
	osListString := "android darwin dragonfly freebsd linux nacl netbsd openbsd plan9 solaris windows"
	osList = strings.Split(osListString, " ")

	archListString := "386 amd64 amd64p32 arm armbe arm64 arm64be ppc64 ppc64le mips mipsle mips64 mips64le mips64p32 mips64p32le ppc s390 s390x sparc sparc64"
	archList = strings.Split(archListString, " ")
}

// IterativeScan attempts to obtain a list of imported dependencies from a
// package. This scanning is different from ImportDir as part of the go/build
// package. It looks over different permutations of the supported OS/Arch to
// try and find all imports. This is different from setting UseAllFiles to
// true on the build Context. It scopes down to just the supported OS/Arch.
//
// Note, there are cases where multiple packages are in the same directory. This
// usually happens with an example that has a main package and a +build tag
// of ignore. This is a bit of a hack. It causes UseAllFiles to have errors.
func IterativeScan(path string) ([]string, error) {

	// TODO(mattfarina): Add support for release tags.

	tgs, _ := readBuildTags(path)
	// Handle the case of scanning with no tags
	tgs = append(tgs, "")

	var pkgs []string
	for _, tt := range tgs {

		// split the tag combination to look at permutations.
		ts := strings.Split(tt, ",")
		var ttgs []string
		var arch string
		var ops string
		for _, ttt := range ts {
			dirty := false
			if strings.HasPrefix(ttt, "!") {
				dirty = true
				ttt = strings.TrimPrefix(ttt, "!")
			}
			if isSupportedOs(ttt) {
				if dirty {
					ops = getOsValue(ttt)
				} else {
					ops = ttt
				}
			} else if isSupportedArch(ttt) {
				if dirty {
					arch = getArchValue(ttt)
				} else {
					arch = ttt
				}
			} else {
				if !dirty {
					ttgs = append(ttgs, ttt)
				}
			}
		}

		// Handle the case where there are no tags but we need to iterate
		// on something.
		if len(ttgs) == 0 {
			ttgs = append(ttgs, "")
		}

		b := build.Default

		// Make sure use all files is off
		b.UseAllFiles = false

		// Set the OS and Arch for this pass
		b.GOARCH = arch
		b.GOOS = ops
		b.BuildTags = ttgs
		//msg.Debug("Scanning with Arch(%s), OS(%s), and Build Tags(%v)", arch, ops, ttgs)

		pk, err := b.ImportDir(path, 0)

		// If there are no buildable souce with this permutation we skip it.
		if err != nil && strings.HasPrefix(err.Error(), "no buildable Go source files in") {
			continue
		} else if err != nil && strings.HasPrefix(err.Error(), "found packages ") {
			// A permutation may cause multiple packages to appear. For example,
			// an example file with an ignore build tag. If this happens we
			// ignore it.
			// TODO(mattfarina): Find a better way.
			//msg.Debug("Found multiple packages while scanning %s: %s", path, err)
			continue
		} else if err != nil {
			//msg.Debug("Problem parsing package at %s for %s %s", path, ops, arch)
			return []string{}, err
		}

		for _, dep := range pk.Imports {
			found := false
			for _, p := range pkgs {
				if p == dep {
					found = true
				}
			}
			if !found {
				pkgs = append(pkgs, dep)
			}
		}
	}

	return pkgs, nil
}

func readBuildTags(p string) ([]string, error) {
	_, err := os.Stat(p)
	if err != nil {
		return []string{}, err
	}

	d, err := os.Open(p)
	if err != nil {
		return []string{}, err
	}

	objects, err := d.Readdir(-1)
	if err != nil {
		return []string{}, err
	}

	var tags []string
	for _, obj := range objects {

		// only process Go files
		if strings.HasSuffix(obj.Name(), ".go") {
			fp := filepath.Join(p, obj.Name())

			co, err := readGoContents(fp)
			if err != nil {
				return []string{}, err
			}

			// Only look at places where we had a code comment.
			if len(co) > 0 {
				t := findTags(co)
				for _, tg := range t {
					found := false
					for _, tt := range tags {
						if tt == tg {
							found = true
						}
					}
					if !found {
						tags = append(tags, tg)
					}
				}
			}
		}
	}

	return tags, nil
}

// Read contents of a Go file up to the package declaration. This can be used
// to find the the build tags.
func readGoContents(fp string) ([]byte, error) {
	f, err := os.Open(fp)
	defer f.Close()
	if err != nil {
		return []byte{}, err
	}

	var s scanner.Scanner
	s.Init(f)
	var tok rune
	var pos scanner.Position
	for tok != scanner.EOF {
		tok = s.Scan()

		// Getting the token text will skip comments by default.
		tt := s.TokenText()
		// build tags will not be after the package declaration.
		if tt == "package" {
			pos = s.Position
			break
		}
	}

	buf := bytes.NewBufferString("")
	f.Seek(0, 0)
	_, err = io.CopyN(buf, f, int64(pos.Offset))
	if err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

// From a byte slice of a Go file find the tags.
func findTags(co []byte) []string {
	p := co
	var tgs []string
	for len(p) > 0 {
		line := p
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line, p = line[:i], p[i+1:]
		} else {
			p = p[len(p):]
		}
		line = bytes.TrimSpace(line)
		// Only look at comment lines that are well formed in the Go style
		if bytes.HasPrefix(line, []byte("//")) {
			line = bytes.TrimSpace(line[len([]byte("//")):])
			if len(line) > 0 && line[0] == '+' {
				f := strings.Fields(string(line))

				// We've found a +build tag line.
				if f[0] == "+build" {
					for _, tg := range f[1:] {
						tgs = append(tgs, tg)
					}
				}
			}
		}
	}

	return tgs
}

// Get an OS value that's not the one passed in.
func getOsValue(n string) string {
	for _, o := range osList {
		if o != n {
			return o
		}
	}

	return n
}

func isSupportedOs(n string) bool {
	for _, o := range osList {
		if o == n {
			return true
		}
	}

	return false
}

// Get an Arch value that's not the one passed in.
func getArchValue(n string) string {
	for _, o := range archList {
		if o != n {
			return o
		}
	}

	return n
}

func isSupportedArch(n string) bool {
	for _, o := range archList {
		if o == n {
			return true
		}
	}

	return false
}
