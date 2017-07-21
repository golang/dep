// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows

package gps

import (
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

// killProcess manages the termination of subprocesses in a way that tries to be
// gentle (via os.Interrupt), but resorts to Kill if needed.
//
//
// TODO(sdboyer) should the return differentiate between whether gentle methods
// succeeded vs. falling back to a hard kill?
func killProcess(cmd *exec.Cmd, isDone *int32) error {
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
	// https://github.com/golang/go/issues/19798 . Also cannot rely on it
	// because cmd.ProcessState will be nil before the process exits, and
	// checking if nil create a data race.
	if !atomic.CompareAndSwapInt32(isDone, 1, 1) {
		to := time.NewTimer(3 * time.Second)
		tick := time.NewTicker(50 * time.Millisecond)

		defer to.Stop()
		defer tick.Stop()

		// Loop until the ProcessState shows up, indicating the proc has exited,
		// or the timer expires and
		for !atomic.CompareAndSwapInt32(isDone, 1, 1) {
			select {
			case <-to.C:
				return cmd.Process.Kill()
			case <-tick.C:
			}
		}
	}

	return nil
}
