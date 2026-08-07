package designworkflow

import (
	"context"
	"testing"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/repair"
)

type recordingZoneRefillRunner struct {
	called bool
}

func (runner *recordingZoneRefillRunner) RefillZones(context.Context, checks.KiCadCLI, string, repair.ZoneRefillOptions) (repair.ZoneRefillRunResult, error) {
	runner.called = true
	return repair.ZoneRefillRunResult{}, nil
}

func TestPrepareGeneratedZoneFillSkipsOptionalDraftZone(t *testing.T) {
	runner := &recordingZoneRefillRunner{}
	result := prepareGeneratedZoneFill(context.Background(), Request{
		ExplicitCircuit: &ExplicitCircuitSpec{Zones: []ExplicitZoneSpec{{Net: "GND", Layers: []string{"F.Cu"}}}},
	}, nil, CreateOptions{ZoneRefill: runner})
	if result.Required || result.Ran || len(result.Issues) != 0 {
		t.Fatalf("optional zone fill result = %#v", result)
	}
	if runner.called {
		t.Fatal("optional draft zone invoked KiCad refill")
	}
}

func TestPrepareGeneratedZoneFillRequiresStrictZoneEvidence(t *testing.T) {
	result := prepareGeneratedZoneFill(context.Background(), Request{
		ExplicitCircuit: &ExplicitCircuitSpec{Zones: []ExplicitZoneSpec{{Net: "GND", Layers: []string{"In1.Cu"}}}},
		Validation:      ValidationSpec{StrictZones: true},
	}, nil, CreateOptions{})
	if !result.Required || len(result.Issues) == 0 {
		t.Fatalf("strict zone fill result = %#v", result)
	}
}
