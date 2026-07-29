//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	lockFileFailImmediately = 0x1
	lockFileExclusive       = 0x2
	errorLockViolation      = syscall.Errno(33)
)

var (
	kernel32LockFileEx   = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	kernel32UnlockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")
)

func advisoryLock(file *os.File) error {
	var overlapped syscall.Overlapped
	result, _, callErr := kernel32LockFileEx.Call(
		file.Fd(),
		lockFileFailImmediately|lockFileExclusive,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result != 0 {
		return nil
	}
	if errors.Is(callErr, errorLockViolation) {
		return errAdvisoryLockBusy
	}
	if callErr != nil && callErr != syscall.Errno(0) {
		return callErr
	}
	return syscall.EINVAL
}

func advisoryUnlock(file *os.File) error {
	var overlapped syscall.Overlapped
	result, _, callErr := kernel32UnlockFileEx.Call(
		file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result != 0 {
		return nil
	}
	if callErr != nil && callErr != syscall.Errno(0) {
		return callErr
	}
	return syscall.EINVAL
}

func advisoryLockBusy(err error) bool {
	return errors.Is(err, errAdvisoryLockBusy)
}
