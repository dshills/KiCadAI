//go:build !unix

package promotiontoolchain

import (
	"context"
	"errors"
)

func acquireFileLock(context.Context, string) (func(), error) {
	return nil, errors.New("toolchain bootstrap locking is unsupported on this platform")
}
