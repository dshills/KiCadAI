package designworkflow

import (
	"context"
	"reflect"
	"testing"
	"time"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestAutonomousCorrectionLoopStopsWhenCorrectionNotRequired(t *testing.T) {
	request, placed, routed := genericCorrectionLoopFixture()
	routed.Stage.Issues = nil
	callbackCalls := 0
	_, selected, summary := maybeRetryPlacementRoutingWithRouter(correctionLoopTestContext(t), request, placed, routed, request.RoutingRetry, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		callbackCalls++
		return next, routed
	})
	report := correctionReportFromRoutingStage(t, selected)
	if callbackCalls != 0 || summary.StopReason != CorrectionStopNotRequired || summary.Attempts != 1 || summary.Applied != 0 {
		t.Fatalf("not-required summary = %#v, callback calls=%d", summary, callbackCalls)
	}
	if report.Attempts != 1 || report.PlanEvaluations != 1 || len(report.AttemptHistory) != 2 || report.AttemptHistory[1].Outcome != "plan_rejected" || report.AttemptHistory[1].Plan == nil || report.AttemptHistory[1].Plan.StopReason != CorrectionStopNotRequired {
		t.Fatalf("not-required report = %#v", report)
	}
}

func TestAutonomousCorrectionLoopStopsOnCancellation(t *testing.T) {
	request, placed, routed := genericCorrectionLoopFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, selected, summary := maybeRetryPlacementRoutingWithRouter(ctx, request, placed, routed, request.RoutingRetry, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		t.Fatal("router called after cancellation")
		return next, routed
	})
	report := correctionReportFromRoutingStage(t, selected)
	if summary.StopReason != CorrectionStopContextCanceled || summary.Attempts != 1 || len(report.AttemptHistory) != 1 || report.StopReason != CorrectionStopContextCanceled {
		t.Fatalf("canceled summary=%#v report=%#v", summary, report)
	}
}

func TestAutonomousCorrectionLoopStopsBeforeRoutingRepeatedPlacement(t *testing.T) {
	request, placed, routed := genericCorrectionLoopFixture()
	diagnostic := correctionDiagnostic(CorrectionComponentOverlap, routing.RepairClearance, []string{"J1", "R1"}, []string{"SIG"})
	plan, err := PlanAutonomousCorrection(request, placed.Request, placed.Result.Placements, []AutonomousCorrectionDiagnostic{diagnostic}, AutonomousCorrectionPlanOptions{Attempt: 2, MaxAttempts: request.RoutingRetry.MaxAttempts})
	if err != nil {
		t.Fatal(err)
	}
	adjusted, application, err := ApplyAutonomousCorrectionPlan(request, placed.Request, placed.Result.Placements, plan, nil)
	if err != nil || !application.Applied {
		t.Fatalf("preflight application = %#v err=%v", application, err)
	}
	preview := placeAdjustedRequest(correctionLoopTestContext(t), adjusted)
	if workflowStageBlocked(preview.Stage) {
		t.Fatalf("preflight placement = %#v", preview.Stage.Issues)
	}
	placed.Result = preview.Result
	callbackCalls := 0
	_, selected, summary := maybeRetryPlacementRoutingWithRouter(correctionLoopTestContext(t), request, placed, routed, request.RoutingRetry, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		callbackCalls++
		return next, routed
	})
	report := correctionReportFromRoutingStage(t, selected)
	if callbackCalls != 0 || summary.StopReason != CorrectionStopRepeatedPlacementState || summary.Attempts != 1 || summary.Applied != 0 {
		t.Fatalf("repeated-state summary = %#v, callback calls=%d", summary, callbackCalls)
	}
	if report.PlanEvaluations != 1 || len(report.AttemptHistory) != 2 || report.AttemptHistory[1].Application == nil || !report.AttemptHistory[1].Application.Applied {
		t.Fatalf("repeated-state report = %#v", report)
	}
}

func TestAutonomousCorrectionLoopKeepsInitialOnNonImprovement(t *testing.T) {
	request, placed, routed := genericCorrectionLoopFixture()
	_, selected, summary := maybeRetryPlacementRoutingWithRouter(correctionLoopTestContext(t), request, placed, routed, request.RoutingRetry, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		return next, RoutingStageResult{
			Result: routing.Result{Status: routing.StatusPartial, Metrics: routing.Metrics{FailedNetCount: 1}},
			Stage:  StageResult{Name: StageRouting, Status: StageStatusBlocked},
		}
	})
	report := correctionReportFromRoutingStage(t, selected)
	if summary.StopReason != CorrectionStopNonImprovingRetry || summary.SelectedAttempt != 1 || summary.Attempts != 2 || summary.Applied != 1 {
		t.Fatalf("non-improving summary = %#v", summary)
	}
	if report.SelectedAttempt != 1 || len(report.AttemptHistory) != 2 || !report.AttemptHistory[0].Selected || report.AttemptHistory[1].Selected {
		t.Fatalf("non-improving report = %#v", report)
	}
}

func TestAutonomousCorrectionLoopStopsAtBudgetWithBestAttempt(t *testing.T) {
	request, placed, routed := genericCorrectionLoopFixture()
	request.RoutingRetry.MaxAttempts = 2
	routed.Result.Metrics.FailedNetCount = 2
	_, selected, summary := maybeRetryPlacementRoutingWithRouter(correctionLoopTestContext(t), request, placed, routed, request.RoutingRetry, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		return next, RoutingStageResult{
			Result: routing.Result{Status: routing.StatusPartial, Metrics: routing.Metrics{FailedNetCount: 1}},
			Stage:  StageResult{Name: StageRouting, Status: StageStatusBlocked},
		}
	})
	report := correctionReportFromRoutingStage(t, selected)
	if summary.StopReason != CorrectionStopMaxAttempts || summary.SelectedAttempt != 2 || summary.Attempts != 2 || summary.Applied != 1 {
		t.Fatalf("budget summary = %#v", summary)
	}
	if report.StopReason != CorrectionStopMaxAttempts || report.SelectedAttempt != 2 || len(report.AttemptHistory) != 2 || !report.AttemptHistory[1].Selected {
		t.Fatalf("budget report = %#v", report)
	}
}

func TestAutonomousCorrectionLoopAppliesRoutingOnlyPlanWithoutCallingPlacerRouter(t *testing.T) {
	request, placed, _ := genericCorrectionLoopFixture()
	operations := []transactions.Operation{
		correctionRouteOperation(t, "ALPHA", 0, 2, 10, 18, 10),
		correctionRouteOperation(t, "BETA", 1, 10, 4, 10, 18),
		correctionRouteOperation(t, "DECOY", 2, 2, 2, 18, 2),
	}
	routingRequest := correctionSelectiveRoutingRequest()
	issues := routing.ValidatePhysicalClearance(routingRequest, routingRoutesFromOperations(operations))
	if len(issues) == 0 {
		t.Fatal("routing-only loop fixture is not initially blocked")
	}
	routed := RoutingStageResult{
		Request: routingRequest,
		Result: routing.Result{Status: routing.StatusBlocked, Metrics: routing.Metrics{
			NetCount: 3, RoutedNetCount: 1, FailedNetCount: 2,
		}},
		Operations: operations,
		Stage:      NewStageResult(StageRouting, issues),
	}
	callbackCalls := 0
	_, selected, summary := maybeRetryPlacementRoutingWithRouter(correctionLoopTestContext(t), request, placed, routed, request.RoutingRetry, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		callbackCalls++
		return next, routed
	})
	if callbackCalls != 0 {
		t.Fatalf("routing-only correction invoked full placement/router callback %d times", callbackCalls)
	}
	if summary.StopReason != CorrectionStopRouted || summary.SelectedAttempt != 2 || summary.Attempts != 2 || summary.Applied != 1 {
		t.Fatalf("routing-only retry summary = %#v", summary)
	}
	report := correctionReportFromRoutingStage(t, selected)
	if report.SelectedAttempt != 2 || len(report.AttemptHistory) != 2 || report.AttemptHistory[1].Application == nil || !report.AttemptHistory[1].Application.Applied {
		t.Fatalf("routing-only correction report = %#v", report)
	}
	preserved := autonomousCorrectionPreservedOperations(selected.Operations, map[string]struct{}{"ALPHA": {}, "BETA": {}})
	if len(preserved) != 1 || !reflect.DeepEqual(preserved[0], operations[2]) {
		t.Fatalf("routing-only loop changed decoy route: %#v", preserved)
	}
}

func genericCorrectionLoopFixture() (Request, PlacementStageResult, RoutingStageResult) {
	request := correctionExplicitRequest()
	request.RoutingRetry = RoutingRetryPolicySpec{
		Enabled: true, MaxAttempts: GenericAutonomousCorrectionMaxAttempts,
		PreserveFixed: true, StopOnNewBlockers: true, StopOnRepeatedSignature: true, StopOnNonImprovement: true,
	}
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
	return request, placed, routed
}

func correctionReportFromRoutingStage(t *testing.T, routed RoutingStageResult) *AutonomousCorrectionReport {
	t.Helper()
	value, ok := routed.Stage.Summary["autonomous_correction"]
	if !ok {
		t.Fatalf("routing stage missing autonomous correction report: %#v", routed.Stage.Summary)
	}
	report, ok := value.(*AutonomousCorrectionReport)
	if !ok || report == nil {
		t.Fatalf("autonomous correction report type = %T", value)
	}
	return report
}

func correctionLoopTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
