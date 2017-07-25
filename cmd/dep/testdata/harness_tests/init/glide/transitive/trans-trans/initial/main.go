// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/chriswhelix/deptestglide1"
	"github.com/chriswhelix/deptestglide2"
)

func main() {
	var x deptestglide1.Foo
	var y deptestglide2.Foo
	fmt.Println(x)
	fmt.Println(y)
}
