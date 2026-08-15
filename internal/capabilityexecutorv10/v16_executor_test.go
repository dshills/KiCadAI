package capabilityexecutorv10

import "testing"

func TestNewV16IsComplete(t *testing.T) {
	executor := NewV16()
	if executor.decode == nil || executor.synthesize == nil || executor.promote == nil || executor.observe == nil {
		t.Fatal("V16 executor is incomplete")
	}
}
