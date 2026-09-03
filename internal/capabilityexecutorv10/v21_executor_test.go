package capabilityexecutorv10

import (
	"reflect"
	"runtime"
	"testing"
)

func TestNewV21BindsVersionIsolatedSynthesisAndObserver(t *testing.T) {
	v20 := NewV20()
	v21 := NewV21()
	v20Name := runtime.FuncForPC(reflect.ValueOf(v20.synthesize).Pointer()).Name()
	v21Name := runtime.FuncForPC(reflect.ValueOf(v21.synthesize).Pointer()).Name()
	if v20Name == v21Name || v21Name != "kicadai/internal/opentopologysynthesis.SynthesizeV21" {
		t.Fatalf("synthesis bindings = V20 %q V21 %q", v20Name, v21Name)
	}
	observerName := runtime.FuncForPC(reflect.ValueOf(v21.observe).Pointer()).Name()
	if observerName != "kicadai/internal/capabilityfeedback.ObserveRealizabilityAwareV21" {
		t.Fatalf("V21 observer = %q", observerName)
	}
}
