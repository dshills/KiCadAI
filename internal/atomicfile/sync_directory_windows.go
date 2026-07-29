//go:build windows

package atomicfile

// MoveFileExW uses MOVEFILE_WRITE_THROUGH for replacement durability. Windows
// does not expose a portable directory-fsync equivalent.
func syncDirectory(string) error {
	return nil
}
