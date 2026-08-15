package capabilityexecutorv10

import (
	"reflect"
	"runtime"
	"testing"
)

func TestNewV18BindsOnlyV18Synthesis(t *testing.T) {
	v17 := NewV17()
	v18 := NewV18()
	if v18.decode == nil || v18.synthesize == nil || v18.promote == nil || v18.observe == nil {
		t.Fatal("V18 executor is incomplete")
	}
	v17Name := runtime.FuncForPC(reflect.ValueOf(v17.synthesize).Pointer()).Name()
	v18Name := runtime.FuncForPC(reflect.ValueOf(v18.synthesize).Pointer()).Name()
	if v17Name == v18Name || v18Name != "kicadai/internal/opentopologysynthesis.SynthesizeV18" {
		t.Fatalf("synthesis bindings = V17 %q V18 %q", v17Name, v18Name)
	}
}
