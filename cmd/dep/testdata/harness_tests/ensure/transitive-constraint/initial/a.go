// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package a

import (
	"fmt"
	"github.com/carolynvs/deptest-transcons-b"
)

func A() {
	fmt.Println("a did a thing")
	b.B()
}
