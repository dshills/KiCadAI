//go:build windows

package corpusfreeze

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Windows does not provide openat. Resolve both paths, require containment,
// and retain openVerifiedRegular's inode comparison as a fail-closed fallback.
func openRegularFileUnder(root, relative string) (*os.File, error) {
	if relative == "" {
		return nil, fmt.Errorf("empty trusted relative path")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	resolvedPath, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return nil, err
	}
	contained, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("resolved path escapes trusted root")
	}
	return openVerifiedRegular(resolvedPath)
}
