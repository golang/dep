// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !darwin,!freebsd,!linux,!netbsd,!openbsd

package utils

import (
	"fmt"
)

// FileDescriptorLimit returns the current file descriptor limit set in the OS
// TODO: add implementations for all OS, especially Windows
func FileDescriptorLimit() (uint64, error) {
	return 0, fmt.Errorf("Unable to get FD limit on this operating system")
}
