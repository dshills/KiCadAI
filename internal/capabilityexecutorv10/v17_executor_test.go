package capabilityexecutorv10

import "testing"

func TestNewV17IsComplete(t *testing.T) {
	executor := NewV17()
	if executor.decode == nil || executor.synthesize == nil || executor.promote == nil || executor.observe == nil {
		t.Fatal("V17 executor is incomplete")
	}
}
