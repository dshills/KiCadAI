package capabilityexecutorv10

import (
	"reflect"
	"runtime"
	"testing"
)

func TestNewV20BindsOnlyV20Synthesis(t *testing.T) {
	v19 := NewV19()
	v20 := NewV20()
	if v20.decode == nil || v20.synthesize == nil || v20.promote == nil || v20.observe == nil {
		t.Fatal("V20 executor is incomplete")
	}
	v19Name := runtime.FuncForPC(reflect.ValueOf(v19.synthesize).Pointer()).Name()
	v20Name := runtime.FuncForPC(reflect.ValueOf(v20.synthesize).Pointer()).Name()
	if v19Name == v20Name || v20Name != "kicadai/internal/opentopologysynthesis.SynthesizeV20" {
		t.Fatalf("synthesis bindings = V19 %q V20 %q", v19Name, v20Name)
	}
}
