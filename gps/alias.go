package gps

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

//ResourceAlias map of source to alias
type ResourceAlias = map[string]string

//AliasConfigFile alias file location
const AliasConfigFile = "./Gopkg.alias"

// readAlias read alias form  a config file in work directory
func readAlias() (m ResourceAlias, err error) {
	var (
		file   *os.File
		part   []byte
		prefix bool
		lines  []string
	)
	m = make(map[string]string)
	if file, err = os.Open(AliasConfigFile); err != nil {
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
		pair := strings.Split(line, "=")
		if len(pair) != 2 {
			return m, errors.New(fmt.Sprintf("failed to parse alias config in file %s:%d ", AliasConfigFile, l))
		}
		m[pair[0]] = pair[1]
	}
	return
}

// parseAlias checks if a pkg is in alias config
func parseAlias(path string, uri *url.URL) (pd pathDeduction, ok bool) {
	alias, e := readAlias()
	if e != nil {
		if os.IsNotExist(e) {

		} else {
			_, _ = fmt.Fprintf(os.Stderr, "error process alias file %s", AliasConfigFile)
		}
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
