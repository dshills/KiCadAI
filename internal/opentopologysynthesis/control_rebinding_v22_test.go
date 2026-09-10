package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/simulationadmission"
)

// Hand-authored independent fixture, not a corpus requirement or synthesis
// result: a 12 V powered monitor with a fixed 0.75 V input and a 12.5 kohm load.
func testControlBindingFixtureV22(t *testing.T) (Requirement, CandidateGraph, PrimitiveInventory, SimulationEnvironment, simulationadmission.Environment) {
	t.Helper()
	inventory, environment := testHeldOutSynthesisEnvironment(t) // Catalog/model loader only; reads no corpus.
	r := Requirement{Schema: RequirementSchema, Version: RequirementVersion, Project: Project{Name: "control_binding_regression", Title: "Independent control binding regression", Description: "Monitor a fixed input with bounded output noise and voltage error under a twelve-volt supply."},
		Requirements: Requirements{
			Domains: []Domain{
				{ID: "common", Kind: "reference", Source: "external", MinVoltageV: graphFloat(0), NominalVoltageV: graphFloat(0), MaxVoltageV: graphFloat(0)},
				{ID: "rail", Kind: "supply", Source: "external", MinVoltageV: graphFloat(12), NominalVoltageV: graphFloat(12), MaxVoltageV: graphFloat(12), MaxCurrentA: graphFloat(0.1)},
			},
			Ports: []Port{
				{ID: "common", Kind: "reference", Direction: "bidirectional", Domain: "common"},
				{ID: "rail", Kind: "power", Direction: "sink", Domain: "rail"},
				{ID: "stimulus", Kind: "analog_voltage", Direction: "sink", Domain: "common", Electrical: Electrical{MinVoltageV: graphFloat(0.75), NominalVoltageV: graphFloat(0.75), MaxVoltageV: graphFloat(0.75)}},
				{ID: "reading", Kind: "analog_voltage", Direction: "source", Domain: "common", Electrical: Electrical{MinVoltageV: graphFloat(0.7), NominalVoltageV: graphFloat(0.75), MaxVoltageV: graphFloat(0.8)}},
			},
			OperatingCases: []OperatingCase{{ID: "steady", Conditions: []OperatingCondition{{Axis: "supply_voltage", Target: "rail", Min: 12, Max: 12, Unit: "V"}, {Axis: "input_voltage", Target: "stimulus", Min: 0.75, Max: 0.75, Unit: "V"}}}},
			BehavioralRequirements: []BehavioralAssertion{
				{ID: "noise", Metric: "output_noise_rms", Analysis: "noise", Excitation: &Observation{Kind: "port", ID: "stimulus"}, Observation: Observation{Kind: "port", ID: "reading"}, Max: graphFloat(10e-6), FrequencyHz: graphFloat(1000), Unit: "V_rms", OperatingCases: []string{"steady"}},
				{ID: "reading", Metric: "output_voltage", Analysis: "dc_operating_point", Observation: Observation{Kind: "port", ID: "reading"}, Min: graphFloat(0.74), Max: graphFloat(0.76), Unit: "V", OperatingCases: []string{"steady"}},
			}, Constraints: BoardLimits{MaxComponents: 8, MaxWidthMM: 40, MaxHeightMM: 30},
		},
		Acceptance: Acceptance{RequirePrimitiveOnly: true, RequireTopologySearch: true, RequireSimulation: true, RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true, RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true, RequireRoundTripZeroDiff: true, RequireERC: true, RequireStrictDRC: true, RequireDeterministicReplay: true, RequireFailClosed: true},
	}
	r = Normalize(r)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if issues := Validate(r); len(issues) != 0 {
		t.Fatalf("independent requirement invalid: %s %#v", data, issues)
	}
	graph, issues := InitialGraph(r)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	device, found := primitiveByKey(inventory, "opamp.ti.opa992idbvr.sot23_5|sot23_5")
	if !found {
		t.Fatal("fixture model absent")
	}
	resistor, found := valuePrimitiveAt(inventory, r, requirementAnalysisSet(r), "resistor", 12500)
	if !found {
		t.Fatal("fixture resistor absent")
	}
	graph = AddPrimitive(graph, device, nil, []TerminalConnection{{Terminal: "IN_MINUS", Node: "port_stimulus"}, {Terminal: "IN_PLUS", Node: "port_stimulus"}, {Terminal: "OUT", Node: "port_reading"}, {Terminal: "V_MINUS", Node: "port_common"}, {Terminal: "V_PLUS", Node: "port_rail"}})
	graph = AddPrimitive(graph, resistor, graphFloat(12500), []TerminalConnection{{Terminal: "A", Node: "port_reading"}, {Terminal: "B", Node: "port_common"}})
	graph, err = NormalizeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	source, err := simulationadmission.NewSource("independent", simulationadmission.SourceBundled, environment.ModelRegistry)
	if err != nil {
		t.Fatal(err)
	}
	admission := simulationadmission.Environment{Sources: []simulationadmission.Source{source}, EnabledSolvers: simulationadmission.EnabledBuiltinSolverIDs()}
	return r, graph, inventory, environment, admission
}

func TestControlRebindingV22RestoresIndependentElectricalBehavior(t *testing.T) {
	r, graph, inventory, environment, admission := testControlBindingFixtureV22(t)
	policy := DefaultPolicy()
	initial := EvaluateCandidateV20(context.Background(), r, graph, nil, inventory, environment, admission, policy)
	if initial.Status != SimulationEvaluationFailed {
		t.Fatalf("initial status=%s diagnoses=%+v", initial.Status, initial.Diagnoses)
	}
	feedback := CloneGraph(graph)
	for i := range feedback.Instances {
		for j, terminal := range feedback.Instances[i].Terminals {
			if terminal.Terminal == "IN_MINUS" {
				feedback.Instances[i].Terminals[j].Node = "port_reading"
			}
		}
	}
	feedback, _ = NormalizeGraph(feedback)
	if AnalyzeTopologyV21(r, feedback, inventory).Complete {
		t.Fatal("frozen V21 cycle boundary changed")
	}
	feedbackResult := EvaluateCandidateV20(context.Background(), r, feedback, nil, inventory, environment, admission, policy)
	if feedbackResult.Status != SimulationEvaluationPassed {
		t.Fatal("independent feedback counterexample no longer passes electrically")
	}
	batch, err := controlRebindingsV22(context.Background(), r, graph, inventory, 16)
	if err != nil {
		t.Fatal(err)
	}
	passed := 0
	for _, proposal := range batch.candidates {
		result := EvaluateCandidateV20(context.Background(), r, proposal.graph, nil, inventory, environment, admission, policy)
		certificate, certificateErr := certifyElectricalRebindingV22(context.Background(), r, graph, proposal.graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), proposal.change, result)
		if result.Status == SimulationEvaluationPassed {
			if certificateErr != nil || certificate.Hash == "" || len(certificate.Feedback) == 0 || len(certificate.AdmissionHashes) == 0 {
				t.Fatalf("passing repair lacks certificate: %v %+v", certificateErr, certificate)
			}
			passed++
			for _, attempt := range result.Attempts {
				if !attempt.AssertionPass || len(attempt.ModelEvidenceSHA256s) < 4 {
					t.Fatal("passing repair lacks admitted electrical evidence")
				}
			}
			t.Logf("passing control repair: %+v; attempts=%d", proposal.change, len(result.Attempts))
		} else if certificateErr == nil {
			t.Fatal("failed numerical candidate received an electrical certificate")
		}
	}
	if passed == 0 {
		t.Fatalf("no passing independent control repair: candidates=%d work=%d", len(batch.candidates), batch.work)
	}
}
