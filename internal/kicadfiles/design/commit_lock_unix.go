//go:build !windows

package design

import (
	"os"

	"golang.org/x/sys/unix"
)

type directoryCommitLock struct {
	file *os.File
}

func acquireDirectoryCommitLock(path string) (*directoryCommitLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &directoryCommitLock{file: file}, nil
}

func (lock *directoryCommitLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
