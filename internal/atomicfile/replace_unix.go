//go:build !windows

package atomicfile

import "os"

func replace(source string, destination string) error {
	return os.Rename(source, destination)
}
