// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cycle

import (
	"cycle/one"

	"github.com/golang/dep/gps"
)

var (
	A = gps.Solve
	B = one.A
)
