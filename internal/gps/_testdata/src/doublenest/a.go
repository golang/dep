// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package base

import (
	"go/parser"

	"github.com/golang/dep/internal/gps"
)

var (
	_ = parser.ParseFile
	_ = gps.Solve
)
