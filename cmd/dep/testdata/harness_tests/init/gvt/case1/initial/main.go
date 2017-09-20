// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/sdboyer/deptest"
	"github.com/sdboyer/deptestdos"
	"gopkg.in/yaml.v2"
)

func main() {
	var a deptestdos.Bar
	var b yaml.MapItem
	var c deptest.Foo
	fmt.Println(a, b, c)
}
