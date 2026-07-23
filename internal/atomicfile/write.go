// Package atomicfile provides durable same-filesystem file replacement.
package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// Write replaces path with data using a temporary file in the same directory.
func Write(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".atomic-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replace(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	if runtime.GOOS != "windows" {
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
	}
	return nil
}

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

func replace(source string, destination string) error {
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = 6
	}
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		if err = os.Rename(source, destination); err == nil {
			return nil
		}
		if attempt+1 < attempts {
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
		}
	}
	return err
}
