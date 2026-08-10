//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package corpusfreeze

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// openRegularFileUnder opens every component relative to an already opened
// directory descriptor. O_NOFOLLOW on the root, intermediate directories,
// and leaf makes symlink replacement races fail in the kernel.
func openRegularFileUnder(root, relative string) (*os.File, error) {
	if relative == "" {
		return nil, fmt.Errorf("empty trusted relative path")
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open trusted root: %w", err)
	}
	currentFD := rootFD
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		} else {
			flags |= unix.O_NONBLOCK
		}
		nextFD, openErr := unix.Openat(currentFD, part, flags, 0)
		if openErr != nil {
			if currentFD != rootFD {
				_ = unix.Close(currentFD)
			}
			_ = unix.Close(rootFD)
			return nil, fmt.Errorf("open trusted path component: %w", openErr)
		}
		if currentFD != rootFD {
			_ = unix.Close(currentFD)
		}
		currentFD = nextFD
	}
	_ = unix.Close(rootFD)
	file := os.NewFile(uintptr(currentFD), relative)
	if file == nil {
		_ = unix.Close(currentFD)
		return nil, fmt.Errorf("wrap trusted file descriptor")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("not a regular file")
	}
	return file, nil
}
