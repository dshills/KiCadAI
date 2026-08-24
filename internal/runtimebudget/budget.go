// Package runtimebudget centralizes bounded in-process worker concurrency.
package runtimebudget

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const environmentVariable = "KICADAI_MAX_WORKERS"

var (
	initialize sync.Once
	capacity   int
)

// Capacity returns the process-wide worker budget. It defaults to GOMAXPROCS
// and may be reduced, but not increased beyond GOMAXPROCS, by
// KICADAI_MAX_WORKERS.
func Capacity() int {
	initialize.Do(func() {
		capacity = configuredCapacity(os.Getenv(environmentVariable), runtime.GOMAXPROCS(0))
	})
	return capacity
}

func configuredCapacity(value string, processors int) int {
	processors = max(1, processors)
	value = strings.TrimSpace(value)
	if value == "" {
		return processors
	}
	configured, err := strconv.Atoi(value)
	if err != nil || configured <= 0 {
		// Environment validation is exposed separately so command entry points
		// can fail closed. Library code retains a safe single-worker fallback.
		return 1
	}
	return min(configured, processors)
}

// Validate reports invalid explicit configuration.
func Validate() error {
	value := strings.TrimSpace(os.Getenv(environmentVariable))
	if value == "" {
		return nil
	}
	configured, err := strconv.Atoi(value)
	if err != nil || configured <= 0 {
		return fmt.Errorf("invalid %s value %q: want a positive integer", environmentVariable, value)
	}
	return nil
}

// Limit bounds requested workers by available work, optional local caps, and
// the shared process budget. It returns zero when workItems is zero.
func Limit(workItems, requested int, localCaps ...int) int {
	if workItems <= 0 {
		return 0
	}
	workers := min(workItems, max(1, requested), Capacity())
	for _, localCap := range localCaps {
		if localCap > 0 {
			workers = min(workers, localCap)
		}
	}
	return max(1, workers)
}

// NestedLimit reserves capacity for a known inner worker fan-out. This avoids
// multiplying outer plan workers by inner analysis workers.
func NestedLimit(workItems, requested, innerFanout int, localCaps ...int) int {
	innerFanout = max(1, innerFanout)
	outerCap := max(1, Capacity()/innerFanout)
	localCaps = append(localCaps, outerCap)
	return Limit(workItems, requested, localCaps...)
}
