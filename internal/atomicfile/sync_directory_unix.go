//go:build !windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
)

func syncDirectory(path string) (err error) {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := directory.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if err := directory.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.ENOSYS) {
		return err
	}
	return nil
}
