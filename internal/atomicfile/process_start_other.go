//go:build !linux

package atomicfile

func processStartIdentity(int) string {
	return ""
}
