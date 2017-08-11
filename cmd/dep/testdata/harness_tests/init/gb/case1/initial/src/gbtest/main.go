// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	cdt "github.com/carolynvs/deptest"
	"github.com/carolynvs/deptest-subpkg/subby"
	"github.com/sdboyer/deptest"
	"github.com/sdboyer/deptestdos"
)

func main() {
	_ = deptestdos.Bar{}
	_ = deptest.Foo{}
	_ = cdt.Foo{}
	_ = subby.SayHi()
}
