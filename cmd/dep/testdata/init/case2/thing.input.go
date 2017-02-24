// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/golang/notexist/foo/bar"
	"github.com/sdboyer/deptestdos"
)

func main() {
	err := nil
	if err != nil {
		deptestdos.diMeLo(bar.Qux)
	}
}
