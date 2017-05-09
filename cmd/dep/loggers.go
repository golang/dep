// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "log"

// Loggers holds standard loggers and a verbosity flag.
type Loggers struct {
	Out, Err *log.Logger
	// Whether verbose logging is enabled.
	Verbose bool
}
