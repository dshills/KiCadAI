package designworkflow

import (
	"reflect"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestPlanAutonomousCorrectionSelectsSupportedActions(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	tests := []struct {
		name       string
		diagnostic AutonomousCorrectionDiagnostic
		want       AutonomousCorrectionActionKind
	}{
		{name: "spacing", diagnostic: correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"}), want: CorrectionActionAdjustRelativeSpacing},
		{name: "fanout", diagnostic: correctionDiagnostic(CorrectionInaccessiblePad, routing.RepairPadAccess, []string{"R1"}, []string{"SIG"}), want: CorrectionActionImproveEndpointFanout},
		{name: "edge", diagnostic: correctionDiagnostic(CorrectionBlockedEscapeDirection, routing.RepairBoardBoundary, []string{"R1"}, []string{"SIG"}), want: CorrectionActionMoveWithinRegion},
		{name: "distance", diagnostic: correctionDiagnostic(CorrectionRequiredNetDisconnectedEndpoint, routing.RepairConnectivity, []string{"J1", "R1"}, []string{"SIG"}), want: CorrectionActionReduceEndpointDistance},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{test.diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
			if err != nil {
				t.Fatal(err)
			}
			if !plan.Authorized || plan.StopReason != "" || len(plan.Actions) != 1 || plan.Actions[0].Kind != test.want || plan.RetryKey == "" {
				t.Fatalf("plan = %#v, want authorized %s", plan, test.want)
			}
		})
	}
}

func TestPlanAutonomousCorrectionAddsRouteTreeRebuild(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	diagnostic := correctionDiagnostic(CorrectionSameNetBranchMerge, routing.RepairConnectivity, []string{"J1", "R1"}, []string{"SIG"})
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authorized || len(plan.Actions) != 2 || plan.Actions[0].Kind != CorrectionActionImproveEndpointFanout || plan.Actions[1].Kind != CorrectionActionRebuildRouteTree {
		t.Fatalf("route-tree plan = %#v", plan)
	}
}

func TestPlanAutonomousCorrectionStopsFailClosed(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	unsupported := correctionDiagnostic(CorrectionMissingLayerTransition, routing.RepairLayerAccess, []string{"R1"}, []string{"SIG"})
	unsupported.AutomaticAction = false
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{unsupported}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil || plan.Authorized || plan.StopReason != CorrectionStopUnsupportedDiagnostic {
		t.Fatalf("unsupported plan = %#v err=%v", plan, err)
	}

	spacing := correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"})
	distance := correctionDiagnostic(CorrectionRequiredNetDisconnectedEndpoint, routing.RepairConnectivity, []string{"J1", "R1"}, []string{"SIG"})
	plan, err = PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{spacing, distance}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil || plan.Authorized || plan.StopReason != CorrectionStopAmbiguousDiagnostics {
		t.Fatalf("ambiguous plan = %#v err=%v", plan, err)
	}

	plan, err = PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{spacing}, AutonomousCorrectionPlanOptions{Attempt: 4, MaxAttempts: 3})
	if err != nil || plan.StopReason != CorrectionStopBudgetExhausted {
		t.Fatalf("budget plan = %#v err=%v", plan, err)
	}
}

func TestPlanAutonomousCorrectionStopsForFixedAndRepeatedState(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(true)
	diagnostic := correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"})
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil || plan.StopReason != CorrectionStopFixedConstraintConflict {
		t.Fatalf("fixed plan = %#v err=%v", plan, err)
	}

	placementRequest, placements = correctionPlacementState(false)
	plan, err = PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil || !plan.Authorized {
		t.Fatalf("initial plan = %#v err=%v", plan, err)
	}
	repeated, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 3, MaxAttempts: 3, AppliedRetryKeys: []string{plan.RetryKey}})
	if err != nil || repeated.Authorized || repeated.StopReason != CorrectionStopRepeatedRetryKey {
		t.Fatalf("repeated plan = %#v err=%v", repeated, err)
	}
}

func TestPlanAutonomousCorrectionDoesNotMutateInputs(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := correctionPlacementState(false)
	diagnostics := []AutonomousCorrectionDiagnostic{correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"})}
	wantRequest := NormalizeRequest(request)
	wantPlacement := placement.CloneRequest(placementRequest)
	wantPlacements := append([]placement.PlacementResult(nil), placements...)
	wantDiagnostics := append([]AutonomousCorrectionDiagnostic(nil), diagnostics...)
	if _, err := PlanAutonomousCorrection(request, placementRequest, placements, diagnostics, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(NormalizeRequest(request), wantRequest) || !reflect.DeepEqual(placementRequest, wantPlacement) || !reflect.DeepEqual(placements, wantPlacements) || !reflect.DeepEqual(diagnostics, wantDiagnostics) {
		t.Fatal("planning mutated an input")
	}
}

func correctionDiagnostic(category AutonomousCorrectionCategory, source routing.RepairCategory, refs, nets []string) AutonomousCorrectionDiagnostic {
	diagnostic := AutonomousCorrectionDiagnostic{
		Category: category, Source: "routing", SourceCategory: source, SourceAction: routing.ActionMoveComponents,
		IssueCode: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
		Refs: correctionSortedStrings(refs), Nets: correctionSortedStrings(nets), AutomaticAction: true,
	}
	return diagnostic
}

func correctionPlacementState(fixed bool) (placement.Request, []placement.PlacementResult) {
	components := []placement.Component{
		{Ref: "J1", Fixed: fixed, Mobility: placement.MobilityPolicy{Class: placement.MobilityFixed, RouteHandling: placement.RouteHandlingPreserveFixed}},
		{Ref: "R1", Fixed: fixed, Mobility: placement.MobilityPolicy{Class: placement.MobilitySoftPreferred, RouteHandling: placement.RouteHandlingInvalidateRebuild}},
	}
	if !fixed {
		components[0].Mobility = placement.MobilityPolicy{Class: placement.MobilitySoftPreferred, RouteHandling: placement.RouteHandlingInvalidateRebuild}
	}
	request := placement.Request{
		Board:      placement.BoardPlacementArea{WidthMM: 40, HeightMM: 30, MarginMM: 1},
		Components: components,
		Nets:       []placement.Net{{Name: "SIG", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "R1", Pin: "1"}}}},
	}
	placements := []placement.PlacementResult{
		{Ref: "J1", Fixed: fixed, Position: placement.Placement{XMM: 5, YMM: 5, Layer: "F.Cu"}},
		{Ref: "R1", Fixed: fixed, Position: placement.Placement{XMM: 10, YMM: 5, Layer: "F.Cu"}},
	}
	return placement.NormalizeRequest(request), placements
}
