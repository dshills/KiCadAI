package designworkflow

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
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

func TestMaybeRetryPlacementRoutingAttachesGenericCorrectionReport(t *testing.T) {
	request := correctionExplicitRequest()
	request.RoutingRetry = RoutingRetryPolicySpec{Enabled: true, MaxAttempts: 3, StopOnRepeatedSignature: true, StopOnNonImprovement: true}
	placementRequest, placements := validCorrectionPlacementState(false)
	placed := PlacementStageResult{
		Request: placementRequest,
		Result:  placement.Result{Status: placement.StatusPlaced, Placements: placements},
		Stage:   StageResult{Name: StagePlacement, Status: StageStatusOK, Summary: map[string]any{"placed_count": 2}},
	}
	routed := RoutingStageResult{
		Result: routing.Result{Status: routing.StatusRouted, Metrics: routing.Metrics{RoutedNetCount: 1}},
		Stage:  StageResult{Name: StageRouting, Status: StageStatusOK, Summary: map[string]any{"status": "routed"}},
	}
	_, routed, summary := maybeRetryPlacementRouting(context.Background(), request, PCBFragmentResult{}, placed, routed, RoutingOptions{}, request.RoutingRetry)
	if summary.StopReason != "routed" {
		t.Fatalf("retry summary = %#v", summary)
	}
	value, ok := routed.Stage.Summary["autonomous_correction"]
	if !ok {
		t.Fatalf("routing summary missing correction report: %#v", routed.Stage.Summary)
	}
	report, ok := value.(*AutonomousCorrectionReport)
	if !ok || report.StopReason != "routed" || report.SelectedAttempt != 1 || len(report.AttemptHistory) != 1 {
		t.Fatalf("correction report = %#v", value)
	}
}

func TestAutonomousCorrectionEvidenceRecordsPreRoutingBlock(t *testing.T) {
	request := correctionExplicitRequest()
	request.RoutingRetry = RoutingRetryPolicySpec{Enabled: true, MaxAttempts: GenericAutonomousCorrectionMaxAttempts}
	workflow := WorkflowResult{Stages: []StageResult{
		{Name: StagePlacement, Status: StageStatusBlocked},
		{Name: StageRouting, Status: StageStatusSkipped},
	}}
	report, ok := AutonomousCorrectionEvidence(request, workflow)
	if !ok || report.StopReason != "blocked_before_routing" || report.Attempts != 0 || report.MaxAttempts != GenericAutonomousCorrectionMaxAttempts {
		t.Fatalf("pre-routing correction evidence = %#v, ok=%v", report, ok)
	}
	if !report.ProtectedInvariantsPreserved || !report.AllAttemptInvariantsPreserved || report.InitialInvariantFingerprint == "" || report.FinalInvariantFingerprint != report.InitialInvariantFingerprint {
		t.Fatalf("pre-routing invariant evidence = %#v", report)
	}
}

func TestAutonomousCorrectionReportSurvivesWorkflowJSONRoundTrip(t *testing.T) {
	want := AutonomousCorrectionReport{SchemaVersion: AutonomousCorrectionSchemaV1, Scope: "generic-circuit-v1", Enabled: true, MaxAttempts: GenericAutonomousCorrectionMaxAttempts}
	workflow := WorkflowResult{Stages: []StageResult{{Name: StageRouting, Summary: map[string]any{"autonomous_correction": &want}}}}
	data, err := json.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	var decoded WorkflowResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	got, ok := AutonomousCorrectionReportFromWorkflow(decoded)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip report = %#v, ok=%v, want %#v", got, ok, want)
	}
}

func TestMaybeRetryPlacementRoutingRoutesAndSelectsAdjustedPlacement(t *testing.T) {
	request := correctionExplicitRequest()
	request.RoutingRetry = RoutingRetryPolicySpec{Enabled: true, MaxAttempts: 3, StopOnRepeatedSignature: true, StopOnNonImprovement: true}
	placementRequest, placements := validCorrectionPlacementState(false)
	placed := PlacementStageResult{
		Request: placementRequest,
		Result:  placement.Result{Status: placement.StatusPlaced, Placements: placements},
		Stage:   StageResult{Name: StagePlacement, Status: StageStatusOK, Summary: map[string]any{"placed_count": 2}},
	}
	routed := RoutingStageResult{
		Result: routing.Result{Status: routing.StatusPartial, Metrics: routing.Metrics{FailedNetCount: 1}},
		Stage: StageResult{Name: StageRouting, Status: StageStatusBlocked, Issues: []reports.Issue{
			correctionIssue(reports.CodePlacementCollision, "placement.components[0]", []string{"J1", "R1"}, []string{"SIG"}, "collision"),
		}},
	}
	diagnostic := correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"})
	plan, err := PlanAutonomousCorrection(request, placementRequest, placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	adjusted, application, err := ApplyAutonomousCorrectionPlan(request, placementRequest, placements, plan, nil)
	if err != nil || !application.Applied {
		t.Fatalf("preflight correction application = %#v err=%v", application, err)
	}
	if preview := placeAdjustedRequest(context.Background(), adjusted); workflowStageBlocked(preview.Stage) {
		t.Fatalf("preflight adjusted placement blocked: %#v", preview.Stage.Issues)
	}
	callbackCalls := 0
	selectedPlaced, selectedRouted, summary := maybeRetryPlacementRoutingWithRouter(context.Background(), request, placed, routed, request.RoutingRetry, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		callbackCalls++
		if next.Result.Status != placement.StatusPlaced {
			t.Fatalf("retry placement status = %q", next.Result.Status)
		}
		if got, want := next.Request.Rules.ComponentSpacingMM, placementRequest.Rules.ComponentSpacingMM+1; got != want {
			t.Fatalf("retry spacing = %v, want %v", got, want)
		}
		return next, RoutingStageResult{
			Result: routing.Result{Status: routing.StatusRouted, Metrics: routing.Metrics{RoutedNetCount: 1}},
			Stage:  StageResult{Name: StageRouting, Status: StageStatusOK},
		}
	})
	if callbackCalls != 1 || selectedRouted.Result.Status != routing.StatusRouted || selectedPlaced.Result.Status != placement.StatusPlaced {
		t.Fatalf("selected retry: calls=%d placed=%q routed=%q summary=%#v", callbackCalls, selectedPlaced.Result.Status, selectedRouted.Result.Status, summary)
	}
	if selectedPlaced.Request.Rules.ComponentSpacingMM != placementRequest.Rules.ComponentSpacingMM+1 || summary.SelectedAttempt != 2 || summary.Applied != 1 || summary.StopReason != "routed" {
		t.Fatalf("selected placement/request was not retained: placed=%#v summary=%#v", selectedPlaced, summary)
	}
}

func validCorrectionPlacementState(fixed bool) (placement.Request, []placement.PlacementResult) {
	request, placements := correctionPlacementState(fixed)
	for index := range request.Components {
		request.Components[index].FootprintID = "Test:Footprint"
		request.Components[index].Bounds = placement.Bounds{WidthMM: 2, HeightMM: 2, CourtyardMM: 0.25, Source: placement.BoundsExplicit}
		request.Components[index].Pads = []placement.PadSummary{{Name: "1", Net: "SIG", WidthMM: 1, HeightMM: 1, Layers: []string{"F.Cu"}}}
	}
	request.Rules = placement.Rules{GridMM: 0.5, ComponentSpacingMM: 0.5, GroupSpacingMM: 0.5, BoardEdgeClearanceMM: 0.5, PreferTopLayer: true, AllowBackLayer: true}
	return placement.NormalizeRequest(request), placements
}
