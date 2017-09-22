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
	// Grab the caller's context and pass a derived one to CommandContext.
	c := cmd{ctx: ctx}
	ctx, cancel := context.WithCancel(ctx)
	c.Cmd = exec.CommandContext(ctx, name, arg...)
	c.cancel = cancel
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
