package designworkflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/closedloopsynthesis"
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

func TestRunExplicitSimulationRejectsInvalidClosedLoopEvidence(t *testing.T) {
	output := t.TempDir()
	catalogHash := simulationTestHash("catalog")
	plan := trustedRegulatorTestPlan(5)
	plan.CatalogHash = catalogHash
	closedLoop := trustedClosedLoopTestReport(catalogHash)
	stage := runExplicitSimulation(Request{ExplicitCircuit: &ExplicitCircuitSpec{
		CatalogHash: catalogHash, ResolutionHash: closedLoop.SelectedCircuitHash,
		Simulation: &plan, ClosedLoop: &closedLoop,
	}}, output, true)
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

type workflowClosedLoopEvaluator struct{}

func (workflowClosedLoopEvaluator) Evaluate(context.Context, closedloopsynthesis.CandidateState) (closedloopsynthesis.Evaluation, error) {
	simulation := &closedloopsynthesis.SimulationEvidence{}
	evidenceHash, _ := closedloopsynthesis.HashSimulationEvidence(*simulation)
	return closedloopsynthesis.Evaluation{
		EvidenceHash: evidenceHash,
		Simulation:   simulation,
		Measurements: []closedloopsynthesis.Measurement{{RequirementID: "output", OperatingCase: "nominal", Actual: 3.3}},
		ModelDecisions: []closedloopsynthesis.ModelDecision{{
			Component: "r1", Family: "resistor", Claim: simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1},
			Provenance: &simmodel.ModelProvenance{Source: "manufacturer:test", Revision: "a", SHA256: simulationTestHash("model"), ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisDCOperatingPoint}},
			Status:     "used", Reason: "trusted model", RequiredAnalyses: []string{simmodel.AnalysisDCOperatingPoint},
		}},
	}, nil
}

func trustedClosedLoopTestReport(catalogHash string) closedloopsynthesis.Report {
	minimum, maximum := 3.2, 3.4
	requirement := architecturesearch.Requirement{
		Schema: architecturesearch.SchemaIDV3, Version: architecturesearch.VersionV3,
		Requirements: architecturesearch.Requirements{BehavioralRequirements: []architecturesearch.BehavioralRequirement{{
			ID: "output", Metric: "dc_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
			Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimum, Max: &maximum, Unit: "V", OperatingCases: []string{"nominal"}, Critical: true,
		}}},
	}
	input := closedloopsynthesis.Input{
		Requirement: requirement, CatalogHash: catalogHash, FormulaLibraryHash: simulationTestHash("formula"), ModelRegistryHash: simulationTestHash("models"),
		Candidates: []closedloopsynthesis.Candidate{{Fingerprint: simulationTestHash("candidate")}},
	}
	report := closedloopsynthesis.Run(context.Background(), input, workflowClosedLoopEvaluator{}, closedloopsynthesis.DefaultPolicy())
	report.SelectedCircuitHash = simulationTestHash("resolved-circuit")
	return report
}

func simulationTestHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
