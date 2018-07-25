// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd

package utils

import (
	"fmt"
	"syscall"
)

// FileDescriptorLimit returns the current file descriptor limit set in the OS
func FileDescriptorLimit() (uint64, error) {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		return 0, fmt.Errorf("unable to get RLIMIT")
	}
	return rLimit.Cur, nil
}
