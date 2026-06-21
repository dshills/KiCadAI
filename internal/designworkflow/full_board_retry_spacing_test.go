package designworkflow

import (
	"context"
	"slices"
	"testing"
	"time"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestFullBoardRetrySpacingImprovesRoutingEvidence(t *testing.T) {
	metadata := loadFullBoardRetryMetadata(t, "spacing_improves")
	request := loadFullBoardRetryRequestForTest(t, "spacing_improves")
	placed := fullBoardRetrySeedPlacement(t, "spacing_improves")
	initial := fullBoardRetryBlockedRouteSearchResult(2, []string{"SIG"})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	bestPlaced, bestRouted, summary := maybeRetryPlacementRouting(ctx, request, PCBFragmentResult{}, placed, initial, RoutingOptions{Mode: routing.ModeSingleLayer}, request.RoutingRetry)

	assertRetrySummaryInvariant(t, summary, true)
	if summary.Applied < 1 || summary.Attempts < 2 {
		t.Fatalf("summary = %#v", summary)
	}
	if !slices.Contains(summary.HintCategories, string(PlacementRetryIncreaseSpacing)) {
		t.Fatalf("hint categories = %#v", summary.HintCategories)
	}
	if !routingAttemptBetter(bestRouted, initial) {
		t.Fatalf("best route did not improve: best=%#v initial=%#v", bestRouted.Result.Metrics, initial.Result.Metrics)
	}
	if bestRouted.Result.Metrics.FailedNetCount >= initial.Result.Metrics.FailedNetCount &&
		bestRouted.Result.Status == initial.Result.Status {
		t.Fatalf("best route evidence did not improve: best=%#v initial=%#v", bestRouted.Result, initial.Result)
	}
	if diffs := fullBoardRetryConstraintDiff(placed.Request, bestPlaced.Request); len(diffs) != 0 {
		t.Fatalf("hard constraints changed after retry: %#v", diffs)
	}
	if metadata.ExpectedImprovement != "routing_status_or_failed_net_count" {
		t.Fatalf("metadata = %#v", metadata)
	}
	stageSummary, ok := retrySummaryFromStage(t, bestRouted.Stage)
	if !ok || stageSummary.Applied != summary.Applied {
		t.Fatalf("routing stage retry summary = %#v ok=%v, want %#v", stageSummary, ok, summary)
	}
}

func fullBoardRetryBlockedRouteSearchResult(failedNets int, nets []string) RoutingStageResult {
	result := routing.Result{Status: routing.StatusBlocked}
	result.Metrics.FailedNetCount = failedNets
	result.Issues = []reports.Issue{{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Message:  "net is unconnected because no legal route exists",
		Nets:     append([]string(nil), nets...),
	}}
	stage := StageResult{
		Name:   StageRouting,
		Status: StageStatusBlocked,
		Summary: map[string]any{
			"status":      string(routing.StatusBlocked),
			"routed_nets": 0,
			"failed_nets": failedNets,
		},
		Issues: result.Issues,
	}
	return RoutingStageResult{Result: result, Stage: stage}
}
