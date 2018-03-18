// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"bufio"
	"context"
	"fmt"
	"github.com/golang/dep/internal/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

const fileRegistryName = "Gopkg.reg"

type fileRegistry struct {
	locations []*fileRegistryLine
}

type fileRegistryLine struct {
	sourcePattern string
	matcher       *regexp.Regexp
	root          string
	gitSources    []string
}

var _ Deducer = &fileRegistry{}

// NewFileRegistry constructs a default Registry, iff the Gopkg.reg file exists at the
// project root; otherwise, return nil.
func NewFileRegistry(rootPath string) Deducer {
	// Is there a Gopkg.reg file?
	names, err := fs.ReadActualFilenames(rootPath, []string{fileRegistryName})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Problems looking to instantiate the FileRegistry: %v\n", err)
		return nil
	}
	if fn, ok := names[fileRegistryName]; ok {
		// We just load the file contents on creation; let's not be snazzy here.
		reg, err := loadFileRegistry(path.Join(rootPath, fn))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Problems looking to load the FileRegistry: %v\n", err)
			return nil
		}
		return reg
	}
	return nil
}

func loadFileRegistry(fn string) (*fileRegistry, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	locations := []*fileRegistryLine{}

	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), " ")
		re, err2 := regexp.Compile(parts[0])
		if err2 != nil {
			err = err2
			fmt.Fprintln(os.Stderr, "Bad syntax in regexp:", err2)
		} else {
			locations = append(locations, &fileRegistryLine{
				sourcePattern: scanner.Text(),
				matcher:       re,
				root:          parts[1],
				gitSources:    parts[2:],
			})
		}
	}
	return &fileRegistry{locations: locations}, err
}

func (fr *fileRegistry) deduceRootPath(ctx context.Context, path string) (pathDeduction, error) {
	var err error
	for _, frl := range fr.locations {
		if frl.matcher.MatchString(path) {
			mbs := []maybeSource{}
			root := frl.matcher.ReplaceAllString(path, frl.root)
			for _, gs := range frl.gitSources {
				res := frl.matcher.ReplaceAllString(path, gs)
				u, err2 := url.Parse(res)
				if err2 != nil {
					fmt.Fprintln(os.Stderr, "Trouble parsing URL:", err2)
					err = err2
				} else {
					mbs = append(mbs, maybeGitSource{url: u})
				}
			}
			return pathDeduction{
				root: root,
				mb:   mbs,
			}, err
		}
	}
	return pathDeduction{}, errNoKnownPathMatch
}
