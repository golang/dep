// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package foo

import "github.com/sdboyer/deptest"

func Foo() deptest.Foo {
	var y deptest.Foo

	return y
}
