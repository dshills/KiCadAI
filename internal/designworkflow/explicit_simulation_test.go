package designworkflow

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/simmodel"
)

func TestRunExplicitSimulationWritesTrustedRegulatorEvidence(t *testing.T) {
	output := t.TempDir()
	plan := trustedRegulatorTestPlan(5)
	stage := runExplicitSimulation(Request{ExplicitCircuit: &ExplicitCircuitSpec{Simulation: &plan}}, output, true)
	if stage.Status != StageStatusOK || len(stage.Artifacts) != 1 {
		t.Fatalf("simulation stage = %#v", stage)
	}
	if _, err := os.Stat(filepath.Join(output, ExplicitSimulationArtifactPath)); err != nil {
		t.Fatalf("simulation artifact: %v", err)
	}
}

func TestRunExplicitSimulationFailsClosedForInsufficientHeadroom(t *testing.T) {
	plan := trustedRegulatorTestPlan(3.4)
	stage := runExplicitSimulation(Request{ExplicitCircuit: &ExplicitCircuitSpec{Simulation: &plan}}, t.TempDir(), true)
	if stage.Status != StageStatusBlocked || len(stage.Artifacts) != 0 {
		t.Fatalf("simulation stage = %#v", stage)
	}
}

func trustedRegulatorTestPlan(inputVoltage float64) simmodel.Plan {
	return simmodel.Plan{
		RegistryVersion: simmodel.RegistryVersion, RegistryHash: simmodel.RegistryHash(), CatalogID: "test", CatalogHash: "catalog-hash",
		ModelID: simmodel.ModelLinearRegulatorIdealV1,
		Bindings: []simmodel.ResolvedBinding{{Role: "regulator", Component: "regulator", CatalogID: "regulator.test", Family: "regulator", ModelParameters: []simmodel.NamedValue{
			{Name: "max_load_current_ma", Value: 600}, {Name: "min_headroom_v", Value: 0.4}, {Name: "output_voltage_v", Value: 3.3},
		}}},
		Inputs:     []simmodel.NamedValue{{Name: "input_voltage_v", Value: inputVoltage}, {Name: "load_current_ma", Value: 20}},
		Assertions: []simmodel.Assertion{{Metric: "output_voltage_v", Min: 3.2, Max: 3.4}},
	}
}
