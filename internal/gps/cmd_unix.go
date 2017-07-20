// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows

package gps

import (
	"os"
	"os/exec"
	"time"
)

// killProcess manages the termination of subprocesses in a way that tries to be
// gentle (via os.Interrupt), but resorts to Kill if needed.
//
//
// TODO(sdboyer) should the return differentiate between whether gentle methods
// succeeded vs. falling back to a hard kill?
func killProcess(cmd *exec.Cmd) error {
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		// If an error comes back from attempting to signal, proceed immediately
		// to hard kill.
		return cmd.Process.Kill()
	}

	// If the process doesn't exit immediately, check every 50ms, up to 3s,
	// after which send a hard kill.
	//
	// Cannot rely on cmd.ProcessState.Exited() here, as that is not set
	// correctly when the process exits due to a signal. See
	// https://github.com/golang/go/issues/19798
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		to := time.NewTimer(3 * time.Second)
		tick := time.NewTicker(50 * time.Millisecond)

		defer to.Stop()
		defer tick.Stop()

		// Loop until the ProcessState shows up, indicating the proc has exited,
		// or the timer expires and
		for cmd.ProcessState != nil {
			select {
			case <-to.C:
				return cmd.Process.Kill()
			case <-tick.C:
			}
		}
	}

	return nil
}
