package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"testing"
)

func TestV20PreservesIneligibleHistoricalRunByteForByte(t *testing.T) {
	requirement := Requirement{}
	v20Inventory := PrimitiveInventory{Hash: "v20", CatalogHash: "v20", ModelRegistryHash: "v20"}
	v20Environment := SimulationEnvironment{CatalogHash: "v20"}
	v18Inventory := PrimitiveInventory{Hash: "v18", CatalogHash: "v18", ModelRegistryHash: "v18"}
	v18Environment := SimulationEnvironment{CatalogHash: "v18"}
	legacyInventory := PrimitiveInventory{Hash: "legacy", CatalogHash: "legacy", ModelRegistryHash: "legacy"}
	legacyEnvironment := SimulationEnvironment{CatalogHash: "legacy"}
	want := SynthesizeV18WithLegacy(context.Background(), requirement, v18Inventory, v18Environment, legacyInventory, legacyEnvironment, DefaultPolicy())
	got := SynthesizeV20WithLegacy(context.Background(), requirement, v20Inventory, v20Environment, v18Inventory, v18Environment, legacyInventory, legacyEnvironment, DefaultPolicy())
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(wantJSON) || got.Hash != want.Hash {
		t.Fatalf("V20 changed ineligible V18 run\nwant %s\n got %s", wantJSON, gotJSON)
	}
}

func TestV20AdmissionFrontierUsesTypedGenericEvidence(t *testing.T) {
	requirement := Requirement{Requirements: Requirements{BehavioralRequirements: []BehavioralAssertion{{
		ID: "voltage", Metric: "dc_voltage", Analysis: "dc_operating_point",
	}}}}
	run := SynthesisRun{Report: Report{Status: StatusUnsupported}, Candidates: []SynthesisCandidateEvidence{{Evaluations: []SimulationEvaluation{{
		Diagnoses: []Diagnosis{{Code: diagnosisMetricUnsupported, RequirementID: "voltage", Analysis: "dc_operating_point", Metric: "dc_voltage"}},
	}}}}}
	if !analysisAdmissionFrontierV20(requirement, run) {
		t.Fatal("newly registered direct metric was not selected")
	}
	run.Candidates[0].Evaluations[0].Diagnoses[0] = Diagnosis{Code: diagnosisSimulationInvalid, Analysis: "electrothermal"}
	if analysisAdmissionFrontierV20(requirement, run) {
		t.Fatal("nonselected electrothermal capability entered V20")
	}
	run.Candidates[0].Evaluations[0].Diagnoses[0] = Diagnosis{Code: diagnosisSimulationInvalid, Analysis: "stability"}
	if !analysisAdmissionFrontierV20(requirement, run) {
		t.Fatal("selected deterministic solver evidence was not selected")
	}
}
