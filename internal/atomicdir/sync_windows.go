//go:build windows

package atomicdir

// Windows does not expose a portable directory fsync through os.File. File
// contents are synced before MoveFileEx performs the no-replace publication.
func syncDirectory(string) error { return nil }
