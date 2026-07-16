package circuitgraph

import (
	"encoding/json"
	"testing"

	"kicadai/internal/reports"
)

func TestPlanRepairStopsForAmbiguousOrReviewOnlyCandidates(t *testing.T) {
	document := validTestDocument()
	patch := PatchOperation{Op: "replace_endpoint", Net: "IN", Endpoint: &document.Nets[0].Endpoints[0], Replacement: &document.Nets[0].Endpoints[0]}
	first := RepairOption{DiagnosticID: "a", Disposition: "agent_selectable", OperationTemplate: patch}
	second := RepairOption{DiagnosticID: "b", Disposition: "agent_selectable", OperationTemplate: patch}
	if plan := PlanRepair(document, false, []RepairOption{first, second}, nil, nil, 3); plan.State != RepairPlanNeedsReview || plan.StopReason != "ambiguous_repair_options" || plan.Patch != nil {
		t.Fatalf("ambiguous plan = %#v", plan)
	}
	if plan := PlanRepair(document, false, nil, []reports.Issue{{Code: "KICAD_DRC"}}, nil, 3); plan.State != RepairPlanNeedsReview || plan.StopReason != "no_fully_derived_repair" || plan.Patch != nil {
		t.Fatalf("review-only plan = %#v", plan)
	}
}

func TestPlanRepairRejectsInvalidCandidateAndIsDeterministic(t *testing.T) {
	document := validTestDocument()
	invalid := RepairOption{DiagnosticID: "invalid", Disposition: "agent_selectable", OperationTemplate: PatchOperation{Op: "replace_project"}}
	if plan := PlanRepair(document, false, []RepairOption{invalid}, nil, nil, 3); plan.State != RepairPlanBlocked || plan.StopReason != "invalid_candidate_patch" || plan.Patch != nil {
		t.Fatalf("invalid patch plan = %#v", plan)
	}
	first := PlanRepair(document, true, nil, nil, nil, 3)
	second := PlanRepair(document, true, nil, nil, nil, 3)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("non-deterministic plans:\n%s\n%s", firstJSON, secondJSON)
	}
}
