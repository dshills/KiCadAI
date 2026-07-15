//go:build windows

package design

import (
	"os"

	"golang.org/x/sys/windows"
)

type directoryCommitLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireDirectoryCommitLock(path string) (*directoryCommitLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	lock := &directoryCommitLock{file: file}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &lock.overlapped); err != nil {
		_ = file.Close()
		return nil, err
	}
	return lock, nil
}

func (lock *directoryCommitLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := windows.UnlockFileEx(windows.Handle(lock.file.Fd()), 0, 1, 0, &lock.overlapped)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
