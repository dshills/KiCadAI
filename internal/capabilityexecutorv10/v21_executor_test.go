package capabilityexecutorv10

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"kicadai/internal/opentopologysynthesis"
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

func TestBindSelectedV21UsesAuthenticatedRequirementPopulation(t *testing.T) {
	corpus, err := LoadPublicDiscovery(filepath.Join("..", "capabilityfeedback", "testdata", "closed_loop_open_set_v10_corpus"))
	if err != nil {
		t.Fatal(err)
	}
	cases := corpus.Cases[:2]
	makeExecutor := func(reason opentopologysynthesis.StopReason) Executor {
		return Executor{synthesize: func(context.Context, opentopologysynthesis.Requirement, opentopologysynthesis.PrimitiveInventory, opentopologysynthesis.SimulationEnvironment, opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
			return opentopologysynthesis.SynthesisRun{Report: opentopologysynthesis.Report{StopReason: reason}}
		}}
	}
	bound, err := bindSelectedV21(makeExecutor("v21"), makeExecutor("v20"), cases, []string{cases[0].Entry.ID})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := decodeRequirement(cases[0].RequirementSource)
	if err != nil {
		t.Fatal(err)
	}
	preserved, err := decodeRequirement(cases[1].RequirementSource)
	if err != nil {
		t.Fatal(err)
	}
	if got := bound.synthesize(context.Background(), selected, opentopologysynthesis.PrimitiveInventory{}, opentopologysynthesis.SimulationEnvironment{}, opentopologysynthesis.DefaultPolicy()).Report.StopReason; got != "v21" {
		t.Fatalf("selected requirement used %q", got)
	}
	if got := bound.synthesize(context.Background(), preserved, opentopologysynthesis.PrimitiveInventory{}, opentopologysynthesis.SimulationEnvironment{}, opentopologysynthesis.DefaultPolicy()).Report.StopReason; got != "v20" {
		t.Fatalf("ineligible requirement used %q", got)
	}
	if _, err := bindSelectedV21(makeExecutor("v21"), makeExecutor("v20"), cases, []string{"unknown"}); err == nil {
		t.Fatal("unknown selected case was accepted")
	}
	duplicate := append([]CaseInput(nil), cases...)
	duplicate[1].RequirementSource = duplicate[0].RequirementSource
	if _, err := bindSelectedV21(makeExecutor("v21"), makeExecutor("v20"), duplicate, []string{cases[0].Entry.ID}); err == nil {
		t.Fatal("duplicate requirement hash was accepted")
	}
}
