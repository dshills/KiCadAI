package capabilityexecutorv10

import (
	"reflect"
	"testing"

	"kicadai/internal/opentopologysynthesis"
)

func TestNewV14BindsLazySynthesisEntrypoint(t *testing.T) {
	executor := NewV14()
	if executor.synthesize == nil ||
		reflect.ValueOf(executor.synthesize).Pointer() != reflect.ValueOf(opentopologysynthesis.SynthesizeV14).Pointer() {
		t.Fatal("V14 executor is not bound to lazy synthesis")
	}
}
