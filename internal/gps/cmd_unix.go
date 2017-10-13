// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows

package gps

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

type cmd struct {
	// ctx is provided by the caller; SIGINT is sent when it is cancelled.
	ctx context.Context
	// cancel is called when the graceful shutdown timeout expires.
	cancel context.CancelFunc
	Cmd    *exec.Cmd
}

func commandContext(ctx context.Context, name string, arg ...string) cmd {
	// Create a one-off cancellable context for use by the CommandContext, in
	// the event that we have to force a Process.Kill().
	ctx2, cancel := context.WithCancel(context.Background())

	c := cmd{
		Cmd:    exec.CommandContext(ctx2, name, arg...),
		cancel: cancel,
		ctx:    ctx,
	}

	// Force subprocesses into their own process group, rather than being in the
	// same process group as the dep process. Because Ctrl-C sent from a
	// terminal will send the signal to the entire currently running process
	// group, this allows us to directly manage the issuance of signals to
	// subprocesses.
	c.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	return c
}

// CombinedOutput is like (*os/exec.Cmd).CombinedOutput except that it
// terminates subprocesses gently (via os.Interrupt), but resorts to Kill if
// the subprocess fails to exit after 1 minute.
func (c cmd) CombinedOutput() ([]byte, error) {
	// Adapted from (*os/exec.Cmd).CombinedOutput
	if c.Cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.Cmd.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var b bytes.Buffer
	c.Cmd.Stdout = &b
	c.Cmd.Stderr = &b

	if err := c.Cmd.Start(); err != nil {
		return nil, err
	}

	// Adapted from (*os/exec.Cmd).Start
	waitDone := make(chan struct{})
	defer close(waitDone)
	go func() {
		select {
		case <-c.ctx.Done():
			if err := c.Cmd.Process.Signal(os.Interrupt); err != nil {
				// If an error comes back from attempting to signal, proceed
				// immediately to hard kill.
				c.cancel()
			} else {
				stopCancel := time.AfterFunc(time.Minute, c.cancel).Stop
				<-waitDone
				stopCancel()
			}
		case <-waitDone:
		}
	}()

	if err := c.Cmd.Wait(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
