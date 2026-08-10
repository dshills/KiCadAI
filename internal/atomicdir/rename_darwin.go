//go:build darwin

package atomicdir

import "golang.org/x/sys/unix"

func renameNoReplace(oldPath, newPath string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_EXCL)
}
