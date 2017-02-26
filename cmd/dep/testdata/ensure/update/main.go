// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	stuff "github.com/carolynvs/go-dep-test"
	"github.com/pkg/errors"
)

func main() {
	fmt.Println(stuff.Thing)
	TryToDoSomething()
}

func TryToDoSomething() error {
	return errors.New("I tried, Billy. I tried...")
}
