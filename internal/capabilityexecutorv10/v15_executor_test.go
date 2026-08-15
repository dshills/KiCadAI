package capabilityexecutorv10

import "testing"

func TestNewV15IsComplete(t *testing.T) {
	executor := NewV15()
	if executor.decode == nil || executor.synthesize == nil || executor.promote == nil || executor.observe == nil {
		t.Fatal("V15 executor is incomplete")
	}
}
