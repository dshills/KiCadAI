package opentopologysynthesis

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/simulationadmission"
)

func testMultiControlFixtureV22(t *testing.T) (Requirement, CandidateGraph, PrimitiveInventory, SimulationEnvironment, simulationadmission.Environment) {
	t.Helper()
	r, graph, inventory, environment, admission := testControlBindingFixtureV22(t)
	r.Project.Name = "independent_dual_monitor"
	r.Project.Description = "Two independent outputs reproduce the same external input with bounded voltage error; the first also has a noise bound."
	r.Requirements.Ports = append(r.Requirements.Ports, Port{ID: "second", Kind: "analog_voltage", Direction: "source", Domain: "common", Electrical: Electrical{MinVoltageV: graphFloat(.7), NominalVoltageV: graphFloat(.75), MaxVoltageV: graphFloat(.8)}})
	r.Requirements.BehavioralRequirements = append(r.Requirements.BehavioralRequirements, BehavioralAssertion{ID: "second", Metric: "output_voltage", Analysis: "dc_operating_point", Observation: Observation{Kind: "port", ID: "second"}, Min: graphFloat(.74), Max: graphFloat(.76), Unit: "V", OperatingCases: []string{"steady"}})
	r = Normalize(r)
	initial, issues := InitialGraph(r)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	initial.Instances = graph.Instances
	device, found := primitiveByKey(inventory, "opamp.ti.opa992idbvr.sot23_5|sot23_5")
	if !found {
		t.Fatal("independent device absent")
	}
	initial = AddPrimitive(initial, device, nil, []TerminalConnection{{Terminal: "IN_MINUS", Node: "port_stimulus"}, {Terminal: "IN_PLUS", Node: "port_stimulus"}, {Terminal: "OUT", Node: "port_second"}, {Terminal: "V_MINUS", Node: "port_common"}, {Terminal: "V_PLUS", Node: "port_rail"}})
	initial, err := NormalizeGraph(initial)
	if err != nil {
		t.Fatal(err)
	}
	if !AnalyzeTopologyV21(r, initial, inventory).Complete {
		t.Fatal("independent multi-control graph is not complete")
	}
	return r, initial, inventory, environment, admission
}

func TestElectricalRepairV22FindsAndAuthenticatesCompoundControlRepair(t *testing.T) {
	r, graph, inventory, environment, admission := testMultiControlFixtureV22(t)
	ctx := context.Background()
	initial := EvaluateCandidateV20(ctx, r, graph, nil, inventory, environment, admission, DefaultPolicy())
	if initial.Status != SimulationEvaluationFailed {
		t.Fatalf("initial %s", initial.Status)
	}
	limits := DefaultElectricalRepairLimitsV22()
	expected := CloneGraph(graph)
	changes := []GraphChange{}
	for i := range expected.Instances {
		output := ""
		for _, terminal := range expected.Instances[i].Terminals {
			if terminal.Terminal == "OUT" {
				output = terminal.Node
			}
		}
		for j, terminal := range expected.Instances[i].Terminals {
			if terminal.Terminal == "IN_MINUS" {
				changes = append(changes, GraphChange{Kind: "redirect_terminal", Primitive: expected.Instances[i].ID, Terminal: terminal.Terminal, FromNode: terminal.Node, ToNode: output})
				expected.Instances[i].Terminals[j].Node = output
			}
		}
	}
	proof := EvaluateElectricalCandidateV22(ctx, r, expected, inventory, environment, admission, DefaultPolicy())
	if proof.Status != SimulationEvaluationPassed {
		t.Fatalf("independent expected feedback failed: %+v", proof.Diagnoses)
	}
	bindings := deriveFeedbackBindingsV22(r, expected, inventory)
	if structure := analyzeElectricalTopologyV22(r, expected, inventory, bindings); !structure.Complete {
		t.Fatalf("independent expected structure failed: %+v bindings=%+v", structure, bindings)
	}
	if _, err := certifyElectricalPathV22(ctx, r, graph, expected, inventory, environment, simulationadmission.PrepareEnvironment(admission), changes, proof); err != nil {
		t.Fatalf("independent expected certificate: %v", err)
	}
	result := RepairElectricalV22(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), limits)
	if result.Status != RepairSearchPassed || result.Selected == nil {
		want, _ := GraphHash(expected)
		for _, trial := range result.Trials {
			if trial.GraphHash == want || trial.Status == SimulationEvaluationPassed {
				t.Logf("passing/expected trial: %+v", trial)
			}
		}
		t.Logf("expected graph=%s", want)
		t.Fatalf("compound search failed: %s, %d trials, %+v", result.StopReason, len(result.Trials), result.Consumption)
	}
	if len(result.Selected.Certificate.Changes) != 2 || len(result.Selected.Certificate.StepGraphHashes) != 2 || result.Selected.Certificate.AttemptCount != 3 {
		t.Fatalf("compound proof incomplete: %+v", result.Selected.Certificate)
	}
	if err := VerifyElectricalRepairSelectionV22(ctx, result, r, graph, inventory, environment, admission); err != nil {
		t.Fatal(err)
	}
	for _, trial := range result.Trials {
		if trial.Depth > limits.MaxDepth {
			t.Fatal("depth bound exceeded")
		}
	}
	if result.Consumption.MaximumFrontier > limits.BeamWidth || result.BindingWork > limits.MaxBindingWork || result.Consumption.CandidateSimulations > limits.MaxEvaluations || result.Consumption.CornerEvaluations > limits.MaxCorners {
		t.Fatal("repair exceeded budget")
	}
	permuted := CloneGraph(graph)
	slices.Reverse(permuted.Instances)
	slices.Reverse(permuted.Nodes)
	replay := RepairElectricalV22(ctx, r, permuted, initial, inventory, environment, admission, DefaultPolicy(), limits)
	if !reflect.DeepEqual(result, replay) {
		t.Fatal("compound repair is not deterministic")
	}
	certificate := result.Selected.Certificate
	certificate.Changes = slices.Clone(certificate.Changes)
	certificate.Changes[0].FromNode = "port_common"
	certificate.Hash = ""
	certificate.Hash = causalCrossStageHash(certificate)
	if err := verifyElectricalCertificateV22(ctx, certificate, r, graph, result.Selected.Graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), result.Selected.Evaluation); err == nil {
		t.Fatal("rehashed false path accepted")
	}
	limits.MaxDepth = 1
	shallow := RepairElectricalV22(ctx, r, graph, initial, inventory, environment, admission, DefaultPolicy(), limits)
	if shallow.Selected != nil || shallow.Status != RepairSearchExhausted || shallow.StopReason != "repair_depth_exhausted" {
		t.Fatal("one edit falsely completes compound repair")
	}
	t.Logf("compound repair: trials=%d binding_work=%d corners=%d", len(result.Trials), result.BindingWork, result.Consumption.CornerEvaluations)
}

func TestElectricalRepairV22BudgetsCancellationAndInitialEvidence(t *testing.T) {
	r, graph, inventory, environment, admission := testControlBindingFixtureV22(t)
	ctx := context.Background()
	initial := EvaluateCandidateV20(ctx, r, graph, nil, inventory, environment, admission, DefaultPolicy())
	for _, category := range []string{"binding", "evaluation", "corner", "invalid", "evidence", "canceled"} {
		t.Run(category, func(t *testing.T) {
			limits := DefaultElectricalRepairLimitsV22()
			e := initial
			runCtx := ctx
			switch category {
			case "binding":
				limits.MaxBindingWork = 1
			case "evaluation":
				limits.MaxEvaluations = 1
			case "corner":
				limits.MaxCorners = 1
			case "invalid":
				limits.BeamWidth = 0
			case "evidence":
				e.GraphHash = "unrelated"
			case "canceled":
				var cancel context.CancelFunc
				runCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			result := RepairElectricalV22(runCtx, r, graph, e, inventory, environment, admission, DefaultPolicy(), limits)
			if result.BindingWork > limits.MaxBindingWork || result.Consumption.CandidateSimulations > limits.MaxEvaluations || result.Consumption.CornerEvaluations > limits.MaxCorners {
				t.Fatalf("budget exceeded: %+v", result)
			}
			if category == "canceled" && (result.Status != RepairSearchCanceled || result.BindingWork != 0) {
				t.Fatal("canceled call performed work")
			}
			if category == "invalid" || category == "evidence" {
				if result.Status != RepairSearchUnsupported || result.BindingWork != 0 {
					t.Fatal("invalid call performed work")
				}
			}
			if category == "corner" && result.Selected != nil {
				t.Fatal("one corner falsely proved two assertions")
			}
		})
	}
}

func TestElectricalRepairV22PreservesPassedAndCriticalAssertions(t *testing.T) {
	r, _, _, _, _ := testControlBindingFixtureV22(t)
	p := SimulationAttempt{RequirementID: "reading", OperatingCase: "steady", CornerID: "nominal", Status: SimulationEvaluationPassed, AssertionPass: true}
	before := SimulationEvaluation{Status: SimulationEvaluationFailed, Attempts: []SimulationAttempt{p}}
	after := SimulationEvaluation{Status: SimulationEvaluationFailed, Attempts: []SimulationAttempt{p, {RequirementID: "noise", OperatingCase: "steady", CornerID: "nominal", Status: SimulationEvaluationFailed}}}
	if allowed, _ := electricalContinuationAllowedV22(r, before, after); !allowed {
		t.Fatal("newly observed failure incorrectly treated as a regression")
	}
	after.Attempts[0].AssertionPass = false
	if allowed, reason := electricalContinuationAllowedV22(r, before, after); allowed || reason != "previously_passing_corner_lost" {
		t.Fatal("passing assertion regressed")
	}
	after.Attempts = after.Attempts[1:]
	if allowed, _ := electricalContinuationAllowedV22(r, before, after); allowed {
		t.Fatal("lost passing assertion accepted")
	}
	for i := range r.Requirements.BehavioralRequirements {
		if r.Requirements.BehavioralRequirements[i].ID == "noise" {
			r.Requirements.BehavioralRequirements[i].Critical = true
		}
	}
	if allowed, reason := electricalContinuationAllowedV22(r, SimulationEvaluation{}, after); allowed || reason != "critical_failure" {
		t.Fatal("critical failure admitted to beam")
	}
}
