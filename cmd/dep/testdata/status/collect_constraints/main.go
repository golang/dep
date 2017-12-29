// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	_ "github.com/boltdb/bolt"
	_ "github.com/sdboyer/dep-test"
	_ "github.com/sdboyer/deptest"
	_ "github.com/sdboyer/deptestdos"
)

type FooBar int
