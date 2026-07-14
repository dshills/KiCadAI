package designworkflow

import (
	"reflect"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/routing"
)

func TestApplyAutonomousCorrectionPlanAppliesBoundedSpacing(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := validCorrectionPlacementState(false)
	diagnostic := correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"})
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	wantOriginal := placement.CloneRequest(placementRequest)
	adjusted, application, err := ApplyAutonomousCorrectionPlan(request, placementRequest, placements, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !application.Applied || !application.ProtectedInvariantsPreserved || application.StopReason != "" {
		t.Fatalf("application = %#v", application)
	}
	if application.Adjustment.SpacingDeltaMM != 1 || adjusted.Rules.ComponentSpacingMM != placementRequest.Rules.ComponentSpacingMM+1 {
		t.Fatalf("adjustment = %#v rules=%#v", application.Adjustment, adjusted.Rules)
	}
	if application.InvariantFingerprintBefore != application.InvariantFingerprintAfter || application.PlacementInvariantBefore != application.PlacementInvariantAfter {
		t.Fatalf("invariants changed: %#v", application)
	}
	if !reflect.DeepEqual(placementRequest, wantOriginal) {
		t.Fatal("application mutated the original placement request")
	}
}

func TestApplyAutonomousCorrectionPlanAppliesDistanceRule(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := validCorrectionPlacementState(false)
	diagnostic := correctionDiagnostic(CorrectionRequiredNetDisconnectedEndpoint, routing.RepairConnectivity, []string{"J1", "R1"}, []string{"SIG"})
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	adjusted, application, err := ApplyAutonomousCorrectionPlan(request, placementRequest, placements, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !application.Applied || len(application.Adjustment.ProximityRules) != 1 || len(adjusted.ProximityRules) != 1 || adjusted.ProximityRules[0].MaxDistanceMM != placementRetryMaxProximityMM {
		t.Fatalf("distance application = %#v adjusted=%#v", application, adjusted.ProximityRules)
	}
}

func TestApplyAutonomousCorrectionPlanRejectsChangedOrRepeatedState(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := validCorrectionPlacementState(false)
	diagnostic := correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"})
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	changed.ExplicitCircuit.Nets[0].WidthMM = 0.5
	_, application, err := ApplyAutonomousCorrectionPlan(changed, placementRequest, placements, plan, nil)
	if err != nil || application.Applied || application.StopReason != CorrectionStopInvariantMismatch {
		t.Fatalf("changed application = %#v err=%v", application, err)
	}
	_, application, err = ApplyAutonomousCorrectionPlan(request, placementRequest, placements, plan, []string{plan.RetryKey})
	if err != nil || application.Applied || application.StopReason != CorrectionStopRepeatedRetryKey {
		t.Fatalf("repeated application = %#v err=%v", application, err)
	}
}

func TestApplyAutonomousCorrectionPlanRejectsUnauthorizedAndInvalidPlan(t *testing.T) {
	request := correctionExplicitRequest()
	placementRequest, placements := validCorrectionPlacementState(false)
	plan := AutonomousCorrectionPlan{SchemaVersion: AutonomousCorrectionSchemaV1, Attempt: 2, MaxAttempts: 3}
	_, application, err := ApplyAutonomousCorrectionPlan(request, placementRequest, placements, plan, nil)
	if err != nil || application.StopReason != CorrectionStopPlanNotAuthorized {
		t.Fatalf("unauthorized application = %#v err=%v", application, err)
	}

	diagnostic := correctionDiagnostic(CorrectionMissingLayerTransition, routing.RepairLayerAccess, []string{"R1"}, []string{"SIG"})
	diagnostic.AutomaticAction = false
	plan, err = PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	_, application, err = ApplyAutonomousCorrectionPlan(request, placementRequest, placements, plan, nil)
	if err != nil || application.StopReason != CorrectionStopPlanNotAuthorized {
		t.Fatalf("reserved application = %#v err=%v", application, err)
	}
}

func validCorrectionPlacementState(fixed bool) (placement.Request, []placement.PlacementResult) {
	request, placements := correctionPlacementState(fixed)
	for index := range request.Components {
		request.Components[index].Bounds = placement.Bounds{WidthMM: 2, HeightMM: 2, CourtyardMM: 0.25, Source: placement.BoundsExplicit}
		request.Components[index].Pads = []placement.PadSummary{{Name: "1", Net: "SIG", WidthMM: 1, HeightMM: 1, Layers: []string{"F.Cu"}}}
	}
	request.Rules = placement.Rules{GridMM: 0.5, ComponentSpacingMM: 0.5, GroupSpacingMM: 0.5, BoardEdgeClearanceMM: 0.5, PreferTopLayer: true, AllowBackLayer: true}
	return placement.NormalizeRequest(request), placements
}
