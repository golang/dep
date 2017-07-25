// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/sdboyer/deptest"
	"github.com/chriswhelix/deptestglide1"
)


func main() {
	var x deptest.Foo
	var y deptestglide1.Foo
	fmt.Println(x)
	fmt.Println(y)
}
