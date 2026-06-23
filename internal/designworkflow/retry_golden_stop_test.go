package designworkflow

import (
	"context"
	"reflect"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestRetryGoldenContextCanceledStopsBeforeSecondAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	placed := PlacementStageResult{
		Request: retryGoldenPlacementRequest(),
		Result: placement.Result{
			Status: placement.StatusPlaced,
			Placements: []placement.PlacementResult{{
				Ref:      "U1",
				Position: placement.Placement{XMM: 10, YMM: 10, Layer: "F.Cu"},
			}},
		},
		Stage: StageResult{Name: StagePlacement, Status: StageStatusOK},
	}
	routed := RoutingStageResult{
		Result: routing.Result{Status: routing.StatusBlocked},
		Stage:  StageResult{Name: StageRouting, Status: StageStatusBlocked},
	}

	_, bestRouted, summary := maybeRetryPlacementRouting(ctx, Request{}, PCBFragmentResult{}, placed, routed, RoutingOptions{}, RoutingRetryPolicySpec{
		Enabled:     true,
		MaxAttempts: 2,
	})

	assertRetrySummaryInvariant(t, summary, true)
	if summary.StopReason != "context_canceled" || summary.Attempts != 1 || summary.Applied != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if !reflect.DeepEqual(bestRouted.Result, routed.Result) {
		t.Fatalf("best routed result changed after cancellation: got %#v want %#v", bestRouted.Result, routed.Result)
	}
	stageSummary, ok := retrySummaryFromStage(t, bestRouted.Stage)
	if !ok || stageSummary.StopReason != "context_canceled" {
		t.Fatalf("best routed summary = %#v ok=%v", stageSummary, ok)
	}
}

func TestRetryGoldenNonImprovingFixtureRanksBestAttempt(t *testing.T) {
	request := loadRetryFixtureRequest(t, "non_improving")
	if !request.RoutingRetry.Enabled || !request.RoutingRetry.StopOnNonImprovement {
		t.Fatalf("retry policy = %#v", request.RoutingRetry)
	}
	placed := placeAdjustedRequest(context.Background(), retryStopPlacementRequest())
	routed := RoutingStageResult{
		Result: routing.Result{
			Status: routing.StatusBlocked,
			Issues: []reports.Issue{{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Message:  "net is unconnected because no legal route exists",
				Nets:     []string{"VIN"},
			}},
		},
		Stage: StageResult{Name: StageRouting, Status: StageStatusBlocked},
	}

	_, bestRouted, summary := maybeRetryPlacementRouting(context.Background(), request, PCBFragmentResult{}, placed, routed, RoutingOptions{Skip: true}, request.RoutingRetry)
	assertRetrySummaryInvariant(t, summary, true)
	if summary.StopReason != "non_improving_retry" || len(summary.AttemptHistory) != 2 || summary.AttemptHistory[1].Attempt != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	if !reflect.DeepEqual(bestRouted.Result, routed.Result) {
		t.Fatalf("non-improving retry replaced best result: got %#v want %#v", bestRouted.Result, routed.Result)
	}
}

func TestRetryGoldenRepeatedPlacementStateFixtureHashesMovableState(t *testing.T) {
	request := loadRetryFixtureRequest(t, "repeated_state")
	if !request.RoutingRetry.Enabled || request.RoutingRetry.MaxAttempts != 3 {
		t.Fatalf("retry policy = %#v", request.RoutingRetry)
	}
	first := []placement.PlacementResult{
		{Ref: "U1", Position: placement.Placement{XMM: 10, YMM: 8, RotationDeg: 90, Layer: "F.Cu"}},
		{Ref: "R1", Position: placement.Placement{XMM: 12, YMM: 8, Layer: "F.Cu"}},
		{Ref: "J1", Fixed: true, Position: placement.Placement{XMM: 1, YMM: 1, Layer: "F.Cu"}},
	}
	repeated := []placement.PlacementResult{
		{Ref: "J1", Fixed: true, Position: placement.Placement{XMM: 3, YMM: 3, Layer: "F.Cu"}},
		{Ref: "R1", Position: placement.Placement{XMM: 12, YMM: 8, Layer: "F.Cu"}},
		{Ref: "U1", Position: placement.Placement{XMM: 10, YMM: 8, RotationDeg: 90, Layer: "F.Cu"}},
	}
	if placementStateHash(first) != placementStateHash(repeated) {
		t.Fatalf("same movable placement state produced different hashes")
	}
	changedMovable := []placement.PlacementResult{
		{Ref: "U1", Position: placement.Placement{XMM: 10, YMM: 8, RotationDeg: 90, Layer: "F.Cu"}},
		{Ref: "R1", Position: placement.Placement{XMM: 13, YMM: 8, Layer: "F.Cu"}},
	}
	if placementStateHash(first) == placementStateHash(changedMovable) {
		t.Fatalf("different movable placement state produced identical hash")
	}
}

func retryStopPlacementRequest() placement.Request {
	request := retryPlacementRequest()
	request.Board = placement.BoardPlacementArea{WidthMM: 100, HeightMM: 80, MarginMM: 2}
	request.Rules.ComponentSpacingMM = 0.5
	request.Rules.GroupSpacingMM = 0.5
	request.Components = append([]placement.Component(nil), request.Components...)
	for index := range request.Components {
		request.Components[index].FootprintID = "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical"
	}
	return request
}
