// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
)

var keyValue *regexp.Regexp = regexp.MustCompile(`^(\s*)(\S+):\s*(\S.+?)\s*$`)
var keyOnly *regexp.Regexp = regexp.MustCompile(`^(\s*)(\S+):\s*$`)
var listItem *regexp.Regexp = regexp.MustCompile(`^(\s*)-\s*(.+?)\s*$`)

type YamlDoc map[string]interface{}
type YamlList []string

type stackItem struct {
	item   interface{}
	key    string
	indent int
}

// Only parses key:val pairs (both strings) and lists of strings
func ParseYaml(rdr io.Reader) (YamlDoc, error) {
	result := make(YamlDoc)
	stack := []stackItem{{result, "", 0}}

	buf := bufio.NewReader(rdr)
	lineNum := 0
	var err error = nil
	var line string
	for err == nil {
		line, err = buf.ReadString('\n')
		lineNum += 1

		// Process Key Only
		m := keyOnly.FindStringSubmatch(line)
		if len(m) > 0 {
			ind, key := len(m[1]), m[2]
			last := &stack[len(stack)-1]
			if ind > last.indent {
				if last.item != nil {
					return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
				}
				n := make(YamlDoc)
				last.item = n
			} else {
				for ind < last.indent {
					if !checkDoc(stack[len(stack)-2].item) {
						return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
					}
					stack[len(stack)-2].item.(YamlDoc)[last.key] = last.item
					stack = stack[:len(stack)-1]
					last = &stack[len(stack)-1]
				}
			}
			stack = append(stack, stackItem{nil, key, ind})
		}

		// Process Key: Value
		m = keyValue.FindStringSubmatch(line)
		if len(m) > 0 {
			ind, key, value := len(m[1]), m[2], m[3]
			last := &stack[len(stack)-1]
			if ind > last.indent {
				if last.item != nil {
					return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
				}
				n := make(YamlDoc)
				last.item = n
				last.indent = ind
			} else {
				for ind < last.indent {
					if !checkDoc(stack[len(stack)-2].item) {
						return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
					}
					stack[len(stack)-2].item.(YamlDoc)[last.key] = last.item
					stack = stack[:len(stack)-1]
					last = &stack[len(stack)-1]
				}
			}
			if !checkDoc(last.item) {
				return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
			}
			last.item.(YamlDoc)[key] = value
		}

		// Process List Item
		m = listItem.FindStringSubmatch(line)
		if len(m) > 0 {
			ind, value := len(m[1]), m[2]
			last := &stack[len(stack)-1]
			if ind > last.indent {
				if last.item != nil {
					return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
				}
				n := make(YamlList, 0)
				last.item = n
				last.indent = ind
			} else {
				for ind < last.indent {
					if !checkDoc(stack[len(stack)-2].item) {
						return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
					}
					stack[len(stack)-2].item.(YamlDoc)[last.key] = last.item
					stack = stack[:len(stack)-1]
					last = &stack[len(stack)-1]
				}
			}
			if !checkList(last.item) {
				return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
			}
			last.item = append(last.item.(YamlList), value)
		}

	}
	if err != io.EOF {
		return nil, err
	}

	last := stack[len(stack)-1]
	for 0 < last.indent {
		if !checkDoc(stack[len(stack)-2].item) {
			return nil, errors.New(fmt.Sprintf("Format error at line %d", lineNum))
		}
		stack[len(stack)-2].item.(YamlDoc)[last.key] = last.item
		stack = stack[:len(stack)-1]
		last = stack[len(stack)-1]
	}
	return result, nil
}

func checkDoc(x interface{}) bool {
	_, ok := x.(YamlDoc)
	return ok
}

func checkList(x interface{}) bool {
	_, ok := x.(YamlList)
	return ok
}

func (x YamlDoc) String() string {
	buff := "\n"
	for key, val := range x {
		buff += fmt.Sprintf("%s: %s\n", key, val)
	}
	return buff
}

func (x YamlList) String() string {
	buff := "\n"
	for _, val := range x {
		buff += fmt.Sprintf("  - %s\n", val)
	}
	return buff
}
