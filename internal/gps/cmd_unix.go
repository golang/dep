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
	cmd    *exec.Cmd
}

func commandContext(ctx context.Context, name string, arg ...string) cmd {
	// Grab the caller's context and pass a derived one to CommandContext.
	c := cmd{ctx: ctx}
	ctx, cancel := context.WithCancel(ctx)
	c.cmd = exec.CommandContext(ctx, name, arg...)
	c.cancel = cancel
	return c
}

func (c cmd) Args() []string {
	return c.cmd.Args
}

func (c cmd) SetDir(dir string) {
	c.cmd.Dir = dir
}

// CombinedOutput is like (*os/exec.Cmd).CombinedOutput except that it
// terminates subprocesses gently (via os.Interrupt), but resorts to Kill if
// the subprocess fails to exit after 1 minute.
func (c cmd) CombinedOutput() ([]byte, error) {
	// Adapted from (*os/exec.Cmd).CombinedOutput
	if c.cmd.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.cmd.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var b bytes.Buffer
	c.cmd.Stdout = &b
	c.cmd.Stderr = &b
	if err := c.cmd.Start(); err != nil {
		return nil, err
	}

	var t *time.Timer
	defer func() {
		if t != nil {
			t.Stop()
		}
	}()
	// Adapted from (*os/exec.Cmd).Start
	waitDone := make(chan struct{})
	defer close(waitDone)
	go func() {
		select {
		case <-c.ctx.Done():
			if err := c.cmd.Process.Signal(os.Interrupt); err != nil {
				// If an error comes back from attempting to signal, proceed
				// immediately to hard kill.
				c.cancel()
			} else {
				t = time.AfterFunc(time.Minute, c.cancel)
			}
		case <-waitDone:
		}
	}()

	if err := c.cmd.Wait(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
