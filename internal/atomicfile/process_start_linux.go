//go:build linux

package atomicfile

import (
	"os"
	"strconv"
	"strings"
)

func processStartIdentity(pid int) string {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	closeParen := strings.LastIndexByte(string(raw), ')')
	if closeParen < 0 {
		return ""
	}
	fields := strings.Fields(string(raw[closeParen+1:]))
	if len(fields) <= 19 {
		return ""
	}
	return fields[19]
}
