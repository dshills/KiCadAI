package opentopologysynthesis

import (
	"context"
	"reflect"
	"testing"
)

func TestSynthesizeV19DelegatesIneligibleV18RunByteForByte(t *testing.T) {
	requirement := Requirement{}
	v19Inventory := PrimitiveInventory{Hash: "v19"}
	v18Inventory := PrimitiveInventory{Hash: "v18"}
	legacyInventory := PrimitiveInventory{Hash: "legacy", CatalogHash: "legacy", ModelRegistryHash: "legacy"}
	v19Simulation := SimulationEnvironment{CatalogHash: "v19"}
	v18Simulation := SimulationEnvironment{CatalogHash: "v18"}
	legacySimulation := SimulationEnvironment{CatalogHash: "legacy"}
	want := SynthesizeV18WithLegacy(context.Background(), requirement, v18Inventory, v18Simulation, legacyInventory, legacySimulation, DefaultPolicy())
	got := SynthesizeV19WithLegacy(context.Background(), requirement, v19Inventory, v19Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation, DefaultPolicy())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("V19 changed an ineligible V18 run: got=%#v want=%#v", got, want)
	}
}

func TestSynthesizeV19PreservesCanceledV18RunByteForByte(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	inventory := causalV19Inventory(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	want := SynthesizeV18(ctx, requirement, inventory, SimulationEnvironment{}, DefaultPolicy())
	got := SynthesizeV19(ctx, requirement, inventory, SimulationEnvironment{}, DefaultPolicy())
	if !reflect.DeepEqual(got, want) || got.Report.Status != StatusCanceled {
		t.Fatalf("V19 canceled delegation = status=%s equal=%t", got.Report.Status, reflect.DeepEqual(got, want))
	}
}

func TestV19CausalTopologyFrontierIsExactAndUnsafeTerminal(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	run := causalV19FrontierRun(requirement,
		Diagnosis{Code: "assertion_above_maximum", RequirementID: "transfer_bound", Analysis: "ac_sweep", Metric: "voltage_gain"},
		Diagnosis{Code: "assertion_below_minimum", RequirementID: "transfer_bound", Analysis: "transient", Metric: "output_voltage"},
	)
	if !causalTopologyRepairFrontierV19(requirement, run) {
		t.Fatal("exact causal-topology frontier was not admitted")
	}

	universal := causalV19FrontierRun(requirement,
		Diagnosis{Code: "simulation_nonconvergent", RequirementID: "transfer_bound", Analysis: "ac_sweep", Metric: "voltage_gain"},
		Diagnosis{Code: "simulation_nonconvergent", RequirementID: "transfer_bound", Analysis: "ac_sweep", Metric: "voltage_gain"},
	)
	if causalTopologyRepairFrontierV19(requirement, universal) {
		t.Fatal("universal simulation gap was treated as topology repair")
	}

	unsafeRequirement := requirement
	unsafeRequirement.Requirements.BehavioralRequirements = append([]BehavioralAssertion(nil), requirement.Requirements.BehavioralRequirements...)
	unsafeRequirement.Requirements.BehavioralRequirements[0].Critical = true
	unsafe := causalV19FrontierRun(unsafeRequirement,
		Diagnosis{Code: "assertion_above_maximum", RequirementID: "transfer_bound", Analysis: "ac_sweep", Metric: "voltage_gain"},
		Diagnosis{Code: "assertion_below_minimum", RequirementID: "transfer_bound", Analysis: "transient", Metric: "output_voltage"},
	)
	if causalTopologyRepairFrontierV19(unsafeRequirement, unsafe) {
		t.Fatal("unsafe all-critical V18 failure was not terminal")
	}

	unrelated := run
	unrelated.Report.Diagnostics = []Diagnostic{{Code: CodeNoPassingGraph}}
	if causalTopologyRepairFrontierV19(requirement, unrelated) {
		t.Fatal("non-causal typed frontier was admitted")
	}
}

func TestV19ReconstructsHashBoundV18RepairBase(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	inventory := causalV19Inventory(t)
	graph := causalV19FeedForwardGraph(t, requirement)
	var err error
	graph, err = NormalizeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildValueSearchPlan(requirement, graph, inventory, DefaultPolicy())
	if plan.Status != ValuePlanReady {
		t.Fatalf("value plan = %s issues=%#v", plan.Status, plan.Issues)
	}
	enumeration := EnumerateValueTrials(plan, 1)
	if len(enumeration.Trials) != 1 {
		t.Fatalf("value trials = %d", len(enumeration.Trials))
	}
	materialized, err := ApplyValueTrial(graph, enumeration.Trials[0], inventory)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := causalV19BeamEvaluation(t, requirement, inventory, materialized, 10, false)
	run := SynthesisRun{
		Search: TopologySearchResult{Candidates: []TopologyCandidate{{Graph: graph}}},
		Candidates: []SynthesisCandidateEvidence{{
			ValuePlan: plan, Evaluations: []SimulationEvaluation{evaluation},
			Repair: &RepairSearchResult{InitialGraphHash: evaluation.GraphHash, InitialEvaluationHash: evaluation.Hash},
		}},
	}
	base, found := causalRepairBaseV19(requirement, run, inventory)
	if !found || base.candidateIndex != 0 || base.evaluation.Hash != evaluation.Hash || base.trial == nil {
		t.Fatalf("base replay failed: found=%t base=%#v", found, base)
	}
	gotHash, _ := GraphHash(base.graph)
	if gotHash != evaluation.GraphHash {
		t.Fatalf("reconstructed graph hash = %q want %q", gotHash, evaluation.GraphHash)
	}

	run.Candidates[0].Repair.InitialGraphHash = stringsOfSHA256V19(91)[0]
	if _, accepted := causalRepairBaseV19(requirement, run, inventory); accepted {
		t.Fatal("forged V18 base hash was accepted")
	}
}

func causalV19FrontierRun(requirement Requirement, left, right Diagnosis) SynthesisRun {
	evaluation := func(diagnosis Diagnosis) SimulationEvaluation {
		return SimulationEvaluation{Diagnoses: []Diagnosis{diagnosis}}
	}
	return SynthesisRun{
		Report: Report{Status: StatusFailed, StopReason: StopRepairExhausted, Diagnostics: []Diagnostic{{Code: CodeRepairExhausted}}},
		Candidates: []SynthesisCandidateEvidence{
			{Fingerprint: "left", Evaluations: []SimulationEvaluation{evaluation(left)}},
			{Fingerprint: "right", Evaluations: []SimulationEvaluation{evaluation(right)}},
		},
	}
}
