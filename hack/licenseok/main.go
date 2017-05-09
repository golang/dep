// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Checks if all files have the license header, a lot of this is based off
// https://github.com/google/addlicense.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const helpText = `Usage: licenseok [flags] pattern [pattern ...]
This program ensures source code files have copyright license headers
by scanning directory patterns recursively.
The pattern argument can be provided multiple times, and may also refer
to single files.
Flags:
`

const tmpl = `The Go Authors. All rights reserved.
Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file.`

var (
	update bool
)

type file struct {
	path string
	mode os.FileMode
}

func init() {
	flag.BoolVar(&update, "u", false, "modifies all source files in place and avoids adding a license header to any file that already has one.")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, helpText)
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}
}

func main() {
	exitStatus := 0

	// process at most 1000 files in parallel
	ch := make(chan *file, 1000)
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for f := range ch {
			wg.Add(1)
			go func(f *file) {
				b, err := ioutil.ReadFile(f.path)
				if err != nil {
					log.Printf("%s: %v", f.path, err)
					exitStatus = 1
				}

				if !hasLicense(b) {
					if !update {
						fmt.Fprintln(os.Stderr, f.path)
						exitStatus = 1
					} else {
						fmt.Fprintln(os.Stdout, f.path)
						if err := addLicense(b, f.path, f.mode); err != nil {
							log.Printf("%s: %v", f.path, err)
							exitStatus = 1
						}
					}
				}

				wg.Done()
			}(f)
		}
		wg.Wait()
		close(done)
	}()

	for _, d := range flag.Args() {
		walk(ch, d)
	}
	close(ch)
	<-done
	os.Exit(exitStatus)
}

func walk(ch chan<- *file, start string) {
	filepath.Walk(start, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			log.Printf("%s error: %v", path, err)
			return nil
		}
		if fi.IsDir() {
			return nil
		}
		ch <- &file{path, fi.Mode()}
		return nil
	})
}

func addLicense(b []byte, path string, fmode os.FileMode) error {
	var lic []byte
	var err error
	switch filepath.Ext(path) {
	default:
		return nil
	case ".c", ".h":
		lic, err = prefix("/*", " * ", " */")
	case ".js", ".css":
		lic, err = prefix("/**", " * ", " */")
	case ".cc", ".cpp", ".cs", ".go", ".hh", ".hpp", ".java", ".m", ".mm", ".proto", ".rs", ".scala", ".swift", ".dart":
		lic, err = prefix("", "// ", "")
	case ".py", ".sh":
		lic, err = prefix("", "# ", "")
	case ".el", ".lisp":
		lic, err = prefix("", ";; ", "")
	case ".erl":
		lic, err = prefix("", "% ", "")
	case ".hs":
		lic, err = prefix("", "-- ", "")
	case ".html", ".xml":
		lic, err = prefix("<!--", " ", "-->")
	case ".php":
		lic, err = prefix("<?php", "// ", "?>")
	}
	if err != nil || lic == nil {
		return err
	}

	line := hashBang(b)
	if len(line) > 0 {
		b = b[len(line):]
		if line[len(line)-1] != '\n' {
			line = append(line, '\n')
		}
		lic = append(line, lic...)
	}
	b = append(lic, b...)
	return ioutil.WriteFile(path, b, fmode)
}

func hashBang(b []byte) []byte {
	var line []byte
	for _, c := range b {
		line = append(line, c)
		if c == '\n' {
			break
		}
	}
	if bytes.HasPrefix(line, []byte("#!")) {
		return line
	}
	return nil
}

func hasLicense(b []byte) bool {
	n := 100
	if len(b) < 100 {
		n = len(b)
	}
	return bytes.Contains(bytes.ToLower(b[:n]), []byte("copyright"))
}

// prefix will execute a license template and prefix the result with top, middle and bottom.
func prefix(top, mid, bot string) ([]byte, error) {
	buf := bytes.NewBufferString(fmt.Sprintf("Copyright %d %s", time.Now().Year(), tmpl))
	var out bytes.Buffer
	if top != "" {
		out.WriteString(top)
		out.WriteRune('\n')
	}
	out.WriteString(mid)
	for _, c := range buf.Bytes() {
		out.WriteByte(c)
		if c == '\n' {
			out.WriteString(mid)
		}
	}
	if bot != "" {
		out.WriteRune('\n')
		out.WriteString(bot)
	}
	out.Write([]byte{'\n', '\n'})
	return out.Bytes(), nil
}
