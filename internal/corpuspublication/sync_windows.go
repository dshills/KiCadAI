//go:build windows

package corpuspublication

// Windows cannot portably FlushFileBuffers on a directory opened through
// os.Open. Files are synced individually before the atomic MoveFileEx publish.
func syncDirectory(string) error { return nil }
