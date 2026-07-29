//go:build windows

package atomicfile

import (
	"syscall"
	"time"
	"unsafe"
)

const (
	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8
)

var (
	kernel32MoveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")
)

func replace(source string, destination string) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		err = replaceOnce(source, destination)
		if err == nil {
			return nil
		}
		if !transientReplaceError(err) || attempt == 5 {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return err
}

func replaceOnce(source string, destination string) error {
	sourcePath, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPath, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	result, _, callErr := kernel32MoveFileEx.Call(
		uintptr(unsafe.Pointer(sourcePath)),
		uintptr(unsafe.Pointer(destinationPath)),
		moveFileReplaceExisting|moveFileWriteThrough,
	)
	if result == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.EINVAL
	}
	return nil
}

func transientReplaceError(err error) bool {
	errno, ok := err.(syscall.Errno)
	if !ok {
		return false
	}
	return errno == syscall.Errno(5) || errno == syscall.Errno(32) || errno == syscall.Errno(33)
}
