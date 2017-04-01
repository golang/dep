// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/golang/notexist/foo"
	"github.com/sdboyer/deptestdos"
)

func main() {
	var x deptestdos.Bar
	y := foo.FooFunc()

	fmt.Println(x, y)
}
