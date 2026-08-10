//go:build !windows

package corpuspublication

import (
	"fmt"
	"os"
)

func syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open publication directory for sync: %w", err)
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return fmt.Errorf("sync publication directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close publication directory: %w", closeErr)
	}
	return nil
}
