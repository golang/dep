// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"varied/namemismatch"
	"varied/otherpath"
	"varied/simple"
)

var (
	_ = simple.S
	_ = nm.V
	_ = otherpath.O
)
