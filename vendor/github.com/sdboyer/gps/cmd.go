package gps

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
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
// command does not show any activity for more than the specified timeout the
// process will be killed.
func (c *monitoredCmd) run(ctx context.Context) error {
	// Check for cancellation before even starting
	if ctx.Err() != nil {
		return ctx.Err()
	}

	ticker := time.NewTicker(c.timeout)
	done := make(chan error, 1)
	defer ticker.Stop()
	go func() { done <- c.cmd.Run() }()

	for {
		select {
		case <-ticker.C:
			if c.hasTimedOut() {
				// On windows it is apparently (?) possible for the process
				// pointer to become nil without Run() having returned (and
				// thus, passing through the done channel). Guard against this.
				if c.cmd.Process != nil {
					if err := c.cmd.Process.Kill(); err != nil {
						return &killCmdError{err}
					}
				}

				return &timeoutError{c.timeout}
			}
		case <-ctx.Done():
			if c.cmd.Process != nil {
				if err := c.cmd.Process.Kill(); err != nil {
					return &killCmdError{err}
				}
			}
			return ctx.Err()
		case err := <-done:
			return err
		}
	}
}

func (c *monitoredCmd) hasTimedOut() bool {
	t := time.Now().Add(-c.timeout)
	return c.stderr.lastActivity().Before(t) &&
		c.stdout.lastActivity().Before(t)
}

func (c *monitoredCmd) combinedOutput(ctx context.Context) ([]byte, error) {
	if err := c.run(ctx); err != nil {
		return c.stderr.buf.Bytes(), err
	}

	return c.stdout.buf.Bytes(), nil
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
	b.lastActivityStamp = time.Now()
	defer b.Unlock()
	return b.buf.Write(p)
}

func (b *activityBuffer) lastActivity() time.Time {
	b.Lock()
	defer b.Unlock()
	return b.lastActivityStamp
}

type timeoutError struct {
	timeout time.Duration
}

func (e timeoutError) Error() string {
	return fmt.Sprintf("command killed after %s of no activity", e.timeout)
}

type killCmdError struct {
	err error
}

func (e killCmdError) Error() string {
	return fmt.Sprintf("error killing command: %s", e.err)
}

func runFromCwd(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	c := newMonitoredCmd(exec.Command(cmd, args...), 2*time.Minute)
	return c.combinedOutput(ctx)
}

func runFromRepoDir(ctx context.Context, repo vcs.Repo, cmd string, args ...string) ([]byte, error) {
	c := newMonitoredCmd(repo.CmdFromDir(cmd, args...), 2*time.Minute)
	return c.combinedOutput(ctx)
}
