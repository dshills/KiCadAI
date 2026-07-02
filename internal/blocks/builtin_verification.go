package blocks

import (
	"embed"
	"io/fs"
)

//go:embed testdata/verification
var builtinVerificationFS embed.FS

func BuiltinVerificationFS() fs.FS {
	sub, err := fs.Sub(builtinVerificationFS, "testdata/verification")
	if err != nil {
		return builtinVerificationFS
	}
	return sub
}
