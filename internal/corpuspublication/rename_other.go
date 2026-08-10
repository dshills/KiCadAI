//go:build !darwin && !linux && !windows

package corpuspublication

import (
	"fmt"
)

func renameNoReplace(_, _ string) error {
	return fmt.Errorf("atomic no-replace directory publication is unsupported on this platform")
}
