// Copyright 2015 Tim Heckman. All rights reserved.
// Use of this source code is governed by the BSD 3-Clause
// license that can be found in the LICENSE file.

package flock_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/theckman/go-flock"

	. "gopkg.in/check.v1"
)

type TestSuite struct {
	path  string
	flock *flock.Flock
}

var _ = Suite(&TestSuite{})

func Test(t *testing.T) { TestingT(t) }

func (t *TestSuite) SetUpTest(c *C) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "go-flock-")
	c.Assert(err, IsNil)
	c.Assert(tmpFile, Not(IsNil))

	t.path = tmpFile.Name()

	defer os.Remove(t.path)
	tmpFile.Close()

	t.flock = flock.NewFlock(t.path)
}

func (t *TestSuite) TearDownTest(c *C) {
	t.flock.Unlock()
	os.Remove(t.path)
}

func (t *TestSuite) TestNewFlock(c *C) {
	var f *flock.Flock

	f = flock.NewFlock(t.path)
	c.Assert(f, Not(IsNil))
	c.Check(f.Path(), Equals, t.path)
	c.Check(f.Locked(), Equals, false)
}

func (t *TestSuite) TestFlock_Path(c *C) {
	var path string
	path = t.flock.Path()
	c.Check(path, Equals, t.path)
}

func (t *TestSuite) TestFlock_Locked(c *C) {
	var locked bool
	locked = t.flock.Locked()
	c.Check(locked, Equals, false)
}

func (t *TestSuite) TestFlock_String(c *C) {
	var str string
	str = t.flock.String()
	c.Assert(str, Equals, t.path)
}

func (t *TestSuite) TestFlock_TryLock(c *C) {
	c.Assert(t.flock.Locked(), Equals, false)

	var locked bool
	var err error

	locked, err = t.flock.TryLock()
	c.Assert(err, IsNil)
	c.Check(locked, Equals, true)
	c.Check(t.flock.Locked(), Equals, true)

	locked, err = t.flock.TryLock()
	c.Assert(err, IsNil)
	c.Check(locked, Equals, true)

	// make sure we just return false with no error in cases
	// where we would have been blocked
	locked, err = flock.NewFlock(t.path).TryLock()
	c.Assert(err, IsNil)
	c.Check(locked, Equals, false)
}

func (t *TestSuite) TestFlock_Unlock(c *C) {
	var err error

	err = t.flock.Unlock()
	c.Assert(err, IsNil)

	// get a lock for us to unlock
	locked, err := t.flock.TryLock()
	c.Assert(err, IsNil)
	c.Assert(locked, Equals, true)
	c.Assert(t.flock.Locked(), Equals, true)

	_, err = os.Stat(t.path)
	c.Assert(os.IsNotExist(err), Equals, false)

	err = t.flock.Unlock()
	c.Assert(err, IsNil)
	c.Check(t.flock.Locked(), Equals, false)
}

func (t *TestSuite) TestFlock_Lock(c *C) {
	c.Assert(t.flock.Locked(), Equals, false)

	var err error

	err = t.flock.Lock()
	c.Assert(err, IsNil)
	c.Check(t.flock.Locked(), Equals, true)

	// test that the short-circuit works
	err = t.flock.Lock()
	c.Assert(err, IsNil)

	//
	// Test that Lock() is a blocking call
	//
	ch := make(chan error, 2)
	gf := flock.NewFlock(t.path)
	defer gf.Unlock()

	go func(ch chan<- error) {
		ch <- nil
		ch <- gf.Lock()
		close(ch)
	}(ch)

	errCh, ok := <-ch
	c.Assert(ok, Equals, true)
	c.Assert(errCh, IsNil)

	err = t.flock.Unlock()
	c.Assert(err, IsNil)

	errCh, ok = <-ch
	c.Assert(ok, Equals, true)
	c.Assert(errCh, IsNil)
	c.Check(t.flock.Locked(), Equals, false)
	c.Check(gf.Locked(), Equals, true)
}
