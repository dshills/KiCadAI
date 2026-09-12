package main

import (
	"testing"
)

func TestExplicitModeAndUnsealedLiveGates(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"run"}, {"run", "--live", "--output", "unexpected"}, {"prepare"}, {"prepare", "--live", "--output", "unexpected"}, {"run", "--live", "--repo", t.TempDir()}} {
		if err := run(args); err == nil {
			t.Fatalf("unsafe/incomplete invocation accepted: %v", args)
		}
	}
}
