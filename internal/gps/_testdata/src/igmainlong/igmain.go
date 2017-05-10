// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Another comment, which the parser should ignore and still see builds tags

// +build ignore

package main

import "unicode"

var _ = unicode.In
