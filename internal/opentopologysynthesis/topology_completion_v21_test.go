package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/simulationadmission"
)

func TestV21PreservesIneligibleV20RunByteForByte(t *testing.T) {
	requirement := Requirement{}
	inventory := PrimitiveInventory{Hash: "same", CatalogHash: "same", ModelRegistryHash: "same"}
	environment := SimulationEnvironment{CatalogHash: "same"}
	want := SynthesizeV20WithLegacy(context.Background(), requirement, inventory, environment, inventory, environment, inventory, environment, DefaultPolicy())
	got := SynthesizeV21WithLegacy(context.Background(), requirement, inventory, environment, inventory, environment, inventory, environment, inventory, environment, DefaultPolicy())
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(wantJSON) || got.Hash != want.Hash {
		t.Fatalf("V21 changed an ineligible V20 run\nwant %s\n got %s", wantJSON, gotJSON)
	}
}

func TestV21PreservesUnsafeV20RunByteForByte(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	if len(requirement.Requirements.BehavioralRequirements) == 0 {
		t.Fatal("causal V19 test requirement has no behavioral assertion")
	}
	requirement.Requirements.BehavioralRequirements[0].Critical = true
	v20 := causalV19FrontierRun(requirement,
		Diagnosis{Code: "assertion_above_maximum", RequirementID: "transfer_bound", Analysis: "dc_operating_point", Metric: "voltage_gain"},
		Diagnosis{Code: "assertion_below_minimum", RequirementID: "transfer_bound", Analysis: "dc_operating_point", Metric: "voltage_gain"},
	)
	got := synthesizeTopologyCompletionV21(
		context.Background(), requirement, v20, causalV19Inventory(t), SimulationEnvironment{},
		simulationadmission.PrepareEnvironment(simulationadmission.Environment{}), DefaultPolicy(),
	)
	if !reflect.DeepEqual(got, v20) {
		t.Fatalf("V21 changed an unsafe V20 run\nwant %#v\n got %#v", v20, got)
	}
}

func TestV21PreservesDownstreamTopologyDiagnosticByteForByte(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	v20 := causalV19FrontierRun(requirement,
		Diagnosis{Code: "assertion_above_maximum", RequirementID: "transfer_bound", Analysis: "dc_operating_point", Metric: "voltage_gain"},
		Diagnosis{Code: "assertion_below_minimum", RequirementID: "transfer_bound", Analysis: "dc_operating_point", Metric: "voltage_gain"},
	)
	v20.Report.Diagnostics = []Diagnostic{{Code: "SIMULATION_INVALID", Path: "simulation"}}
	got := synthesizeTopologyCompletionV21(
		context.Background(), requirement, v20, causalV19Inventory(t), SimulationEnvironment{},
		simulationadmission.PrepareEnvironment(simulationadmission.Environment{}), DefaultPolicy(),
	)
	if !reflect.DeepEqual(got, v20) {
		t.Fatalf("V21 changed a non-topology root with a downstream topology stop\nwant %#v\n got %#v", v20, got)
	}
}

func TestV21DirectSearchFrontierRequiresTopologyDiagnostics(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	direct := SynthesisRun{Report: Report{
		Status: StatusExhausted, StopReason: StopSearchExhausted,
		Diagnostics: []Diagnostic{{Code: CodeSearchExhausted, Path: "search.policy"}},
	}}
	if !topologyCompletionStatusFrontierV21(requirement, direct) {
		t.Fatal("V21 rejected a direct topology-search frontier")
	}
	direct.Report.Diagnostics = append(direct.Report.Diagnostics, Diagnostic{Code: "SIMULATION_INVALID", Path: "simulation"})
	if topologyCompletionStatusFrontierV21(requirement, direct) {
		t.Fatal("V21 accepted a mixed search and non-topology frontier")
	}
}

func TestV21TopologyInvariantsAcceptDirectionallyCompleteGraph(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	inventory := causalV19Inventory(t)
	graph := causalV19FeedForwardGraph(t, requirement)
	first := AnalyzeTopologyV21(requirement, graph, inventory)
	second := AnalyzeTopologyV21(requirement, graph, inventory)
	if !first.Complete || first.Contradictory || len(first.Obligations) != 0 {
		t.Fatalf("complete feed-forward graph rejected: %+v", first)
	}
	if !reflect.DeepEqual(first, second) || first.Hash == "" {
		t.Fatal("V21 invariant evidence is not deterministic")
	}
}

func TestV21PlannerCompletesMissingObservationPathAcrossWorkerCounts(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	inventory := causalV19Inventory(t)
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph: %#v", issues)
	}
	oneLimits := DefaultTopologyCompletionLimitsV21()
	fourLimits := oneLimits
	fourLimits.Workers = 4
	one := PlanTopologyCompletionV21(context.Background(), requirement, graph, inventory, oneLimits)
	four := PlanTopologyCompletionV21(context.Background(), requirement, graph, inventory, fourLimits)
	if one.Status != "complete" || one.Selected == nil || !one.Selected.Invariant.Complete {
		t.Fatalf("single-worker completion failed: status=%s issues=%#v", one.Status, one.Issues)
	}
	if four.Status != "complete" || four.Selected == nil {
		t.Fatalf("four-worker completion failed: status=%s issues=%#v", four.Status, four.Issues)
	}
	if one.Selected.GraphHash != four.Selected.GraphHash ||
		!reflect.DeepEqual(one.Selected.Operations, four.Selected.Operations) {
		t.Fatalf("worker count changed selected repair\none=%+v\nfour=%+v", one.Selected, four.Selected)
	}
	operation := one.Selected.Operations[0]
	if !operation.Accepted || operation.BeforeGraphHash == "" || operation.AfterGraphHash == "" ||
		operation.ParentStateHash == "" || operation.Obligation.EvidenceHash == "" || operation.WorkConsumed == 0 {
		t.Fatalf("incomplete operation provenance: %+v", operation)
	}
}

func TestV21TopologyInvariantRejectsDrivenReference(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	inventory := causalV19Inventory(t)
	graph := causalV19FeedForwardGraph(t, requirement)
	for instanceIndex := range graph.Instances {
		if graph.Instances[instanceIndex].Kind != "opamp" {
			continue
		}
		for terminalIndex := range graph.Instances[instanceIndex].Terminals {
			if graph.Instances[instanceIndex].Terminals[terminalIndex].Terminal == "OUT" {
				graph.Instances[instanceIndex].Terminals[terminalIndex].Node = "port_signal_return"
			}
		}
	}
	graph, _ = NormalizeGraph(graph)
	report := AnalyzeTopologyV21(requirement, graph, inventory)
	if !report.Contradictory || !topologyHasObligationV21(report, TopologyObligationDirectionV21) {
		t.Fatalf("driven reference was not rejected: %+v", report)
	}
}

func TestV21PlannerFailsClosedWithoutApplicablePrimitive(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	graph, _ := InitialGraph(requirement)
	inventory := PrimitiveInventory{Schema: PrimitiveInventorySchema, Version: PrimitiveInventoryVersion, Primitives: []PrimitiveCandidate{}}
	plan := PlanTopologyCompletionV21(context.Background(), requirement, graph, inventory, DefaultTopologyCompletionLimitsV21())
	if plan.Status != TopologyRejectionNoOperationV21 || len(plan.Issues) != 1 || plan.Issues[0].Code != CodeTopologyNoApplicableV21 {
		t.Fatalf("missing inventory did not fail closed: %+v", plan)
	}
}

func TestV21PlannerEnforcesWorkBoundAndHashDetectsTamper(t *testing.T) {
	requirement := causalV19TwoObservationRequirement()
	inventory := causalV19Inventory(t)
	graph, _ := InitialGraph(requirement)
	limits := DefaultTopologyCompletionLimitsV21()
	limits.MaximumWork = 1
	plan := PlanTopologyCompletionV21(context.Background(), requirement, graph, inventory, limits)
	if plan.Status != "exhausted" || !plan.Consumption.BudgetExhausted || plan.Consumption.WorkConsumed != 1 {
		t.Fatalf("work bound not enforced: %+v", plan.Consumption)
	}
	payload, err := json.Marshal(plan)
	if err != nil || len(payload) == 0 || plan.Hash == "" {
		t.Fatalf("invalid plan evidence: bytes=%d hash=%q err=%v", len(payload), plan.Hash, err)
	}
	tampered := plan
	tampered.Status = "complete"
	tampered.Hash = ""
	if causalCrossStageHash(tampered) == plan.Hash {
		t.Fatal("plan hash did not detect tampering")
	}
}

func TestV21PlannerRejectsUnhashableRequirement(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	notANumber := math.NaN()
	requirement.Requirements.BehavioralRequirements[0].Min = &notANumber
	graph, _ := InitialGraph(requirement)
	report := AnalyzeTopologyV21(requirement, graph, causalV19Inventory(t))
	if !report.Contradictory || !topologyHasObligationV21(report, TopologyObligationInvalidEvidenceV21) {
		t.Fatalf("unhashable invariant evidence was misclassified: %+v", report)
	}
	plan := PlanTopologyCompletionV21(context.Background(), requirement, graph, causalV19Inventory(t), DefaultTopologyCompletionLimitsV21())
	if plan.Status != TopologyRejectionInvalidV21 || len(plan.Issues) != 1 || plan.Issues[0].Code != CodeTopologyInvalidRepairV21 {
		t.Fatalf("unhashable requirement did not fail closed: %+v", plan)
	}
}

func TestV21PlannerHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := PlanTopologyCompletionV21(ctx, causalV19Requirement("voltage_gain", "dc_operating_point"), CandidateGraph{}, causalV19Inventory(t), DefaultTopologyCompletionLimitsV21())
	if plan.Status != string(StatusCanceled) || len(plan.Issues) != 1 || plan.Issues[0].Code != CodeCanceled {
		t.Fatalf("canceled context was not honored: %+v", plan)
	}
}

func TestV21GraphMemoryBoundUsesConservativeEstimate(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	graph, issues := InitialGraph(requirement)
	if len(issues) != 0 || !topologyGraphWithinMemoryV21(graph, DefaultTopologyCompletionLimitsV21().MaximumGraphBytes) {
		t.Fatalf("ordinary initial graph rejected by memory estimate: issues=%#v", issues)
	}
	graph.Nodes[0].SemanticID = strings.Repeat("x", DefaultTopologyCompletionLimitsV21().MaximumGraphBytes)
	if topologyGraphWithinMemoryV21(graph, DefaultTopologyCompletionLimitsV21().MaximumGraphBytes) {
		t.Fatal("oversized graph passed the conservative memory estimate")
	}
}

func TestV21RepairCertifiesAlreadyCompleteCausalGraph(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	inventory := causalV19Inventory(t)
	graph := causalV19FeedForwardGraph(t, requirement)
	initial := causalV19BeamEvaluation(t, requirement, inventory, graph, 0, false)
	repair := RepairCandidateV21(context.Background(), requirement, graph, initial, inventory, SimulationEnvironment{}, simulationadmission.Environment{}, DefaultPolicy())
	if repair.Status != RepairSearchFailed || repair.TopologyCompletionV21 == nil || repair.TopologyCompletionV21.Selected == nil ||
		!repair.TopologyCompletionV21.Selected.Invariant.Complete || len(repair.Issues) != 1 || repair.Issues[0].Code != CodeTopologyCertifiedV21 {
		t.Fatalf("complete causal graph was not certified: %+v", repair)
	}
	if repair.Hash == "" || repair.TopologyCompletionV21.Hash == "" {
		t.Fatal("certification evidence is not hash-bound")
	}
}

func topologyHasObligationV21(report TopologyInvariantReportV21, kind string) bool {
	for _, obligation := range report.Obligations {
		if obligation.Kind == kind {
			return true
		}
	}
	return false
}
