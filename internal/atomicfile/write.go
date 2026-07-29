// Package atomicfile provides durable same-filesystem file replacement.
package atomicfile

import (
	"os"
	"path/filepath"
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
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return nil
}
