// Copyright 2015 Tim Heckman. All rights reserved.
// Use of this source code is governed by the BSD 3-Clause
// license that can be found in the LICENSE file.

// +build windows

package flock

import (
	"syscall"
	"unsafe"
)

var (
	kernel32, _         = syscall.LoadLibrary("kernel32.dll")
	procLockFileEx, _   = syscall.GetProcAddress(kernel32, "LockFileEx")
	procUnlockFileEx, _ = syscall.GetProcAddress(kernel32, "UnlockFileEx")
)

const (
	LOCKFILE_FAIL_IMMEDIATELY = 0x00000001
	LOCKFILE_EXCLUSIVE_LOCK   = 0x00000002
)

// Do the interface allocations only once for common
// Errno values.
const (
	errnoERROR_IO_PENDING = 997
)

var (
	errERROR_IO_PENDING error = syscall.Errno(errnoERROR_IO_PENDING)
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case errnoERROR_IO_PENDING:
		return errERROR_IO_PENDING
	}
	// TODO: add more here, after collecting data on the common
	// error values see on Windows. (perhaps when running
	// all.bat?)
	return e
}

func lockFileEx(handle syscall.Handle, flags uint32, reserved uint32, numberOfBytesToLockLow uint32, numberOfBytesToLockHigh uint32, offset *syscall.Overlapped) (success bool, err error) {
	r1, _, e1 := syscall.Syscall6(
		uintptr(procLockFileEx),
		6,
		uintptr(handle),
		uintptr(flags),
		uintptr(reserved),
		uintptr(numberOfBytesToLockLow),
		uintptr(numberOfBytesToLockHigh),
		uintptr(unsafe.Pointer(offset)))

	success = r1 == 1
	if !success {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func unlockFileEx(handle syscall.Handle, reserved uint32, numberOfBytesToLockLow uint32, numberOfBytesToLockHigh uint32, offset *syscall.Overlapped) (success bool, err error) {
	r1, _, e1 := syscall.Syscall6(
		uintptr(procUnlockFileEx),
		5,
		uintptr(handle),
		uintptr(reserved),
		uintptr(numberOfBytesToLockLow),
		uintptr(numberOfBytesToLockHigh),
		uintptr(unsafe.Pointer(offset)),
		0)

	success = r1 == 1
	if !success {
		if e1 != 0 {
			err = errnoErr(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}
