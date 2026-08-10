// Package atomicdir publishes a fully prepared directory without replacing an
// existing destination.
package atomicdir

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func Publish(destination string, build func(string) error) error {
	if build == nil {
		return fmt.Errorf("atomic directory builder is required")
	}
	destination, err := filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return fmt.Errorf("resolve atomic directory destination: %w", err)
	}
	parent := filepath.Dir(destination)
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("resolve atomic directory parent: %w", err)
	}
	destination = filepath.Join(realParent, filepath.Base(destination))
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("atomic directory destination already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect atomic directory destination: %w", err)
	}
	staging, err := os.MkdirTemp(realParent, "."+filepath.Base(destination)+".staging-")
	if err != nil {
		return fmt.Errorf("create atomic directory staging root: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := build(staging); err != nil {
		return err
	}
	if err := os.Chmod(staging, 0o755); err != nil {
		return fmt.Errorf("set atomic directory root permissions: %w", err)
	}
	if err := syncTree(staging); err != nil {
		return fmt.Errorf("sync atomic directory staging tree: %w", err)
	}
	if err := renameNoReplace(staging, destination); err != nil {
		return fmt.Errorf("commit atomic directory: %w", err)
	}
	committed = true
	if err := syncDirectory(realParent); err != nil {
		return fmt.Errorf("sync atomic directory parent: %w", err)
	}
	return nil
}

func syncTree(root string) error {
	var directories []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic paths are prohibited: %s", path)
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular paths are prohibited: %s", path)
		}
		file, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			return err
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncDirectory(directories[index]); err != nil {
			return err
		}
	}
	return nil
}
