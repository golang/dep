// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build windows

package gps

import "os/exec"

func killProcess(cmd *exec.Cmd, isDone *int32) error {
	// TODO it'd be great if this could be more sophisticated...
	return cmd.Process.Kill()
}
