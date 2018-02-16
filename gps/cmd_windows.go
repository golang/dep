// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"context"
	"os/exec"
)

func commandContext(ctx context.Context, name string, arg ...string) cmd {
	return cmd{ctx: ctx, Cmd: exec.CommandContext(ctx, name, arg...)}
}

func (c cmd) CombinedOutput() ([]byte, error) {
	if release, err := c.acquire(); err != nil {
		return nil, err
	} else if release != nil {
		defer release()
	}
	return c.Cmd.CombinedOutput()
}
