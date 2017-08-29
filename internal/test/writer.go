// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"strings"
	"testing"
	"unicode"
)

// Writer adapts a testing.TB to the io.Writer interface
type Writer struct {
	testing.TB
}

func (t Writer) Write(b []byte) (n int, err error) {
	str := string(b)
	if len(str) == 0 {
		return 0, nil
	}

	for _, part := range strings.Split(str, "\n") {
		str := strings.TrimRightFunc(part, unicode.IsSpace)
		if len(str) != 0 {
			t.Log(str)
		}
	}
	return len(b), err
}
