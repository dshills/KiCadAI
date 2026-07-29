//go:build !windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
)

func advisoryLock(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errAdvisoryLockBusy
	}
	return err
}

func advisoryUnlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func advisoryLockBusy(err error) bool {
	return errors.Is(err, errAdvisoryLockBusy)
}
