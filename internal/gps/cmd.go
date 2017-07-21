// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Masterminds/vcs"
)

// monitoredCmd wraps a cmd and will keep monitoring the process until it
// finishes, the provided context is canceled, or a certain amount of time has
// passed and the command showed no signs of activity.
type monitoredCmd struct {
	cmd     *exec.Cmd
	timeout time.Duration
	stdout  *activityBuffer
	stderr  *activityBuffer
}

// noProgressError indicates that the monitored process was terminated due to
// exceeding exceeding the progress timeout.
type noProgressError struct {
	timeout time.Duration
}

// killCmdError indicates that an error occurred while sending a kill signal to
// the monitored process.
type killCmdError struct {
	err error
}

func newMonitoredCmd(cmd *exec.Cmd, timeout time.Duration) *monitoredCmd {
	stdout, stderr := newActivityBuffer(), newActivityBuffer()
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return &monitoredCmd{
		cmd:     cmd,
		timeout: timeout,
		stdout:  stdout,
		stderr:  stderr,
	}
}

// run will wait for the command to finish and return the error, if any. If the
// command does not show any progress, as indicated by writing to stdout or
// stderr, for more than the specified timeout, the process will be killed.
func (c *monitoredCmd) run(ctx context.Context) error {
	// Check for cancellation before even starting
	if ctx.Err() != nil {
		return ctx.Err()
	}

	err := c.cmd.Start()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(c.timeout)
	defer ticker.Stop()

	// Atomic marker to track proc exit state. Guards against bad channel
	// select receive order, where a tick or context cancellation could come
	// in at the same time as process completion, but one of the former are
	// picked first; in such a case, cmd.Process could(?) be nil by the time we
	// call signal methods on it.
	var isDone *int32 = new(int32)
	done := make(chan error, 1)

	go func() {
		// Wait() can only be called once, so this must act as the completion
		// indicator for both normal *and* signal-induced termination.
		done <- c.cmd.Wait()
		atomic.CompareAndSwapInt32(isDone, 0, 1)
	}()

	var killerr error
selloop:
	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			if !atomic.CompareAndSwapInt32(isDone, 1, 1) && c.hasTimedOut() {
				if err := killProcess(c.cmd, isDone); err != nil {
					killerr = &killCmdError{err}
				} else {
					killerr = &noProgressError{c.timeout}
				}
				break selloop
			}
		case <-ctx.Done():
			if !atomic.CompareAndSwapInt32(isDone, 1, 1) {
				if err := killProcess(c.cmd, isDone); err != nil {
					killerr = &killCmdError{err}
				} else {
					killerr = ctx.Err()
				}
				break selloop
			}
		}
	}

	// This is only reachable on the signal-induced termination path, so block
	// until a message comes through the channel indicating that the command has
	// exited.
	//
	// TODO(sdboyer) if the signaling process errored (resulting in a
	// killCmdError stored in killerr), is it possible that this receive could
	// block forever on some kind of hung process?
	<-done
	return killerr
}

func (c *monitoredCmd) hasTimedOut() bool {
	t := time.Now().Add(-c.timeout)
	return c.stderr.lastActivity().Before(t) &&
		c.stdout.lastActivity().Before(t)
}

func (c *monitoredCmd) combinedOutput(ctx context.Context) ([]byte, error) {
	c.cmd.Stderr = c.stdout
	if err := c.run(ctx); err != nil {
		return c.stdout.Bytes(), err
	}

	return c.stdout.Bytes(), nil
}

// activityBuffer is a buffer that keeps track of the last time a Write
// operation was performed on it.
type activityBuffer struct {
	sync.Mutex
	buf               *bytes.Buffer
	lastActivityStamp time.Time
}

func newActivityBuffer() *activityBuffer {
	return &activityBuffer{
		buf: bytes.NewBuffer(nil),
	}
}

func (b *activityBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()

	b.lastActivityStamp = time.Now()

	return b.buf.Write(p)
}

func (b *activityBuffer) String() string {
	b.Lock()
	defer b.Unlock()

	return b.buf.String()
}

func (b *activityBuffer) Bytes() []byte {
	b.Lock()
	defer b.Unlock()

	return b.buf.Bytes()
}

func (b *activityBuffer) lastActivity() time.Time {
	b.Lock()
	defer b.Unlock()

	return b.lastActivityStamp
}

func (e noProgressError) Error() string {
	return fmt.Sprintf("command killed after %s of no activity", e.timeout)
}

func (e killCmdError) Error() string {
	return fmt.Sprintf("error killing command: %s", e.err)
}

func runFromCwd(ctx context.Context, timeout time.Duration, cmd string, args ...string) ([]byte, error) {
	c := newMonitoredCmd(exec.Command(cmd, args...), timeout)
	return c.combinedOutput(ctx)
}

func runFromRepoDir(ctx context.Context, repo vcs.Repo, timeout time.Duration, cmd string, args ...string) ([]byte, error) {
	c := newMonitoredCmd(repo.CmdFromDir(cmd, args...), timeout)
	return c.combinedOutput(ctx)
}

const (
	// expensiveCmdTimeout is meant to be used in a command that is expensive
	// in terms of computation and we know it will take long or one that uses
	// the network, such as clones, updates, ....
	expensiveCmdTimeout = 2 * time.Minute
	// defaultCmdTimeout is just an umbrella value for all other commands that
	// should not take much.
	defaultCmdTimeout = 10 * time.Second
)
