package gps

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

// monitoredCmd wraps a cmd and will keep monitoring the process until it
// finishes or a certain amount of time has passed and the command showed
// no signs of activity.
type monitoredCmd struct {
	cmd     *exec.Cmd
	timeout time.Duration
	buf     *activityBuffer
}

func newMonitoredCmd(cmd *exec.Cmd, timeout time.Duration) *monitoredCmd {
	buf := newActivityBuffer()
	cmd.Stderr = buf
	cmd.Stdout = buf
	return &monitoredCmd{cmd, timeout, buf}
}

// run will wait for the command to finish and return the error, if any. If the
// command does not show any activity for more than the specified timeout the
// process will be killed.
func (c *monitoredCmd) run() error {
	ticker := time.NewTicker(c.timeout)
	done := make(chan error, 1)
	defer ticker.Stop()
	go func() { done <- c.cmd.Run() }()

	for {
		select {
		case <-ticker.C:
			if c.hasTimedOut() {
				if err := c.cmd.Process.Kill(); err != nil {
					return fmt.Errorf("error killing process after command timed out: %s", err)
				}

				return fmt.Errorf("command timed out after %s of no activity", c.timeout)
			}
		case err := <-done:
			return err
		}
	}
}

func (c *monitoredCmd) hasTimedOut() bool {
	return c.buf.lastActivity.Before(time.Now().Add(-c.timeout))
}

func (c *monitoredCmd) combinedOutput() ([]byte, error) {
	if err := c.run(); err != nil {
		return nil, err
	}

	return c.buf.buf.Bytes(), nil
}

// activityBuffer is a buffer that keeps track of the last time a Write
// operation was performed on it.
type activityBuffer struct {
	buf          *bytes.Buffer
	lastActivity time.Time
}

func newActivityBuffer() *activityBuffer {
	return &activityBuffer{
		buf: bytes.NewBuffer(nil),
	}
}

func (b *activityBuffer) Write(p []byte) (int, error) {
	b.lastActivity = time.Now()
	return b.buf.Write(p)
}
