package designworkflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunExplicitSimulationWritesTrustedRegulatorEvidence(t *testing.T) {
	output := t.TempDir()
	stage := runExplicitSimulation(Request{ExplicitCircuit: &ExplicitCircuitSpec{Simulation: &ExplicitSimulationSpec{
		ModelID: "linear_regulator_ideal_v1", Component: "regulator", InputVoltageV: 5,
		LoadCurrentMA: 20, OutputNominalV: 3.3, OutputMinV: 3.2, OutputMaxV: 3.4,
	}}}, output, true)
	if stage.Status != StageStatusOK || len(stage.Artifacts) != 1 {
		t.Fatalf("simulation stage = %#v", stage)
	}
	if _, err := os.Stat(filepath.Join(output, explicitSimulationArtifactPath)); err != nil {
		t.Fatalf("simulation artifact: %v", err)
	}
}

func TestRunExplicitSimulationFailsClosedForInsufficientHeadroom(t *testing.T) {
	stage := runExplicitSimulation(Request{ExplicitCircuit: &ExplicitCircuitSpec{Simulation: &ExplicitSimulationSpec{
		ModelID: "linear_regulator_ideal_v1", Component: "regulator", InputVoltageV: 3.4,
		LoadCurrentMA: 20, OutputNominalV: 3.3, OutputMinV: 3.2, OutputMaxV: 3.4,
	}}}, t.TempDir(), true)
	if stage.Status != StageStatusBlocked || len(stage.Artifacts) != 0 {
		t.Fatalf("simulation stage = %#v", stage)
	}
}
