// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"os"
)

// Verbose specifies if verbose logging is enabled.
var Verbose bool

func Logf(format string, args ...interface{}) {
	// TODO: something else?
	fmt.Fprintf(os.Stderr, "dep: "+format+"\n", args...)
}

func Vlogf(format string, args ...interface{}) {
	if !Verbose {
		return
	}
	Logf(format, args...)
}
