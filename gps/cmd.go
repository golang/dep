// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"context"
	"fmt"
	"os/exec"
	"os"
)

type ctxKey int

const subProcsSem ctxKey = 0

type sem chan struct{}

// CtxWithCmdLimit returns a copy ctx with a semaphore value limiting the number
// of concurrent sub-processes. Returns an error if n is not positive.
func CtxWithCmdLimit(ctx context.Context, n int) (context.Context, error) {
	if n < 1 {
		return nil, fmt.Errorf("cmd limit must be positive: %d", n)
	}

	return context.WithValue(ctx, subProcsSem, make(sem, n)), nil
}

type cmd struct {
	ctx context.Context
	Cmd *exec.Cmd
}

// acquire blocks to acquire a semaphore (if present), and returns a function to
// release or an error if the context is cancelled first.
// No-ops and returns a nil release func when no semaphore is present.
func (c cmd) acquire() (rel func(), err error) {
	if v := c.ctx.Value(subProcsSem); v != nil {
		s := v.(sem)
		select {
		case s <- struct{}{}:
			rel = func() { <-s }
		case <-c.ctx.Done():
			err = c.ctx.Err()
		}
	}
	return
}

func (c cmd) Args() []string {
	return c.Cmd.Args
}

func (c cmd) SetDir(dir string) {
	c.Cmd.Dir = dir
}

func (c cmd) SetEnv(env []string) {
	c.Cmd.Env = env
}

func init() {
	// For our git repositories, we very much assume a "regular" topology.
	// Therefore, no value for the following variables can be relevant to
	// us. Unsetting globally properly propagates to libraries like
	// github.com/Masterminds/vcs, which cannot make the same assumption in
	// general.
	parasiteGitVars := []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_WORK_TREE"}
	for _, e := range parasiteGitVars {
		os.Unsetenv(e)
	}
}
