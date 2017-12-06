// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

func (c cmd) Args() []string {
	return c.Cmd.Args
}

func (c cmd) SetDir(dir string) {
	c.Cmd.Dir = dir
}

func (c cmd) SetEnv(env []string) {
	c.Cmd.Env = env
}
