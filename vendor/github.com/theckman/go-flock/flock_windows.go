// Copyright 2015 Tim Heckman. All rights reserved.
// Use of this source code is governed by the BSD 3-Clause
// license that can be found in the LICENSE file.

package flock

import (
	"syscall"
)

// Lock is a blocking call to try and take the file lock. It will wait until it
// is able to obtain the exclusive file lock. It's recommended that TryLock() be
// used over this function. This function may block the ability to query the
// current Locked() status due to a RW-mutex lock.
//
// If we are already locked, this function short-circuits and returns immediately
// assuming it can take the mutex lock.
func (f *Flock) Lock() error {
	f.m.Lock()
	defer f.m.Unlock()

	if f.l {
		return nil
	}

	if f.fh == nil {
		if err := f.setFh(); err != nil {
			return err
		}
	}

	if _, err := lockFileEx(syscall.Handle(f.fh.Fd()), LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &syscall.Overlapped{}); err != nil {
		return err
	}

	f.l = true
	return nil
}

// Unlock is a function to unlock the file. This file takes a RW-mutex lock, so
// while it is running the Locked() function will be blocked.
//
// This function short-circuits if we are unlocked already. If not, it calls
// syscall.LOCK_UN on the file and closes the file descriptor It does not remove
// the file from disk. It's up to your application to do.
func (f *Flock) Unlock() error {
	f.m.Lock()
	defer f.m.Unlock()

	// if we aren't locked or if the lockfile instance is nil
	// just return a nil error because we are unlocked
	if !f.l || f.fh == nil {
		return nil
	}

	// mark the file as unlocked
	if _, err := unlockFileEx(syscall.Handle(f.fh.Fd()), 0, 1, 0, &syscall.Overlapped{}); err != nil {
		return err
	}

	f.fh.Close()

	f.l = false
	f.fh = nil

	return nil
}

// TryLock is the preferred function for taking a file lock. This function does
// take a RW-mutex lock before it tries to lock the file, so there is the
// possibility that this function may block for a short time if another goroutine
// is trying to take any action.
//
// The actual file lock is non-blocking. If we are unable to get the exclusive
// file lock, the function will return false instead of waiting for the lock.
// If we get the lock, we also set the *Flock instance as being locked.
func (f *Flock) TryLock() (bool, error) {
	f.m.Lock()
	defer f.m.Unlock()

	if f.l {
		return true, nil
	}

	if f.fh == nil {
		if err := f.setFh(); err != nil {
			return false, err
		}
	}

	_, err := lockFileEx(syscall.Handle(f.fh.Fd()), LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &syscall.Overlapped{})

	switch err {
	case nil:
		f.l = true
		return true, nil
	}

	return false, err
}
