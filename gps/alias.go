// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package gps

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

//ResourceAlias map of source to alias
type ResourceAlias = map[string]string

//AliasFile alias file location
const AliasFile = "./Gopkg.alias"

//AliasEnv alias environment variable name
const AliasEnv = "DEPALIAS"

//AliasComment comment line prefix
const AliasComment = "//"

//AliasEnvSeparator source target split char
const AliasEnvSeparator = "="

// readAlias read alias form  a config file in work directory
func readAlias() (m ResourceAlias, err error) {
	var (
		file   *os.File
		part   []byte
		prefix bool
		lines  []string
		pair   []string
	)
	m = make(map[string]string)
	//read alias env
	alias := os.Getenv(AliasEnv)
	if len(alias) != 0 {
		al := strings.Split(alias, string(os.PathListSeparator))
		if len(al) != 0 {
			for l, value := range al {
				pair = strings.Split(value, AliasEnvSeparator)
				if len(pair) != 2 {
					_, _ = fmt.Fprintf(os.Stderr, "failed to parse alias config in environment variable %s:%d \n%s", AliasEnv, l, alias)
					continue
				}
				m[pair[0]] = pair[1]
			}
		}
	}
	//read alias file
	if file, err = os.Open(AliasFile); err != nil {
		if os.IsNotExist(err) {
			err = nil
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "error process alias file %s", AliasFile)
		return
	}
	reader := bufio.NewReader(file)
	buffer := bytes.NewBuffer(make([]byte, 1024))
	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			break
		}
		buffer.Write(part)
		if !prefix {
			lines = append(lines, buffer.String())
			buffer.Reset()
		}
	}
	if err == io.EOF {
		err = nil
	}
	for l, line := range lines {
		if strings.HasPrefix(strings.TrimPrefix(line, " "), AliasComment) {
			continue
		}
		pair := strings.Split(line, "=")
		if len(pair) != 2 {
			_, _ = fmt.Fprintf(os.Stderr, "failed to parse alias config in file %s:%d ", AliasFile, l)
			continue
		}
		m[pair[0]] = pair[1]
	}
	return
}

// parseAlias checks if a pkg is in alias config
func parseAlias(path string, uri *url.URL) (pd pathDeduction, ok bool) {
	alias, e := readAlias()
	if e != nil {

		return pathDeduction{}, false
	}
	for root, source := range alias {
		if strings.HasPrefix(path, root) {
			ok = true
			i := strings.Index(source, "/")
			uri.Host = source[:i]
			uri.Path = source[i:]
			mb := make(maybeSources, len(gopkginSchemes))
			for k, scheme := range gopkginSchemes {
				u := *uri
				u.Scheme = scheme
				mb[k] = maybeGitSource{url: &u}
			}
			pd = pathDeduction{
				root: root,
				mb:   mb,
			}
		}
	}
	return
}
