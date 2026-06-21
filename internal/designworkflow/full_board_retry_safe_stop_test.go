package designworkflow

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"kicadai/internal/routing"
)

func TestFullBoardRetrySafeStopPreservesBestAttempt(t *testing.T) {
	metadata := loadFullBoardRetryMetadata(t, "safe_stop")
	request := loadFullBoardRetryRequestForTest(t, "safe_stop")
	placed := fullBoardRetrySeedPlacement(t, "safe_stop")
	initial := fullBoardRetryBlockedRouteSearchResult(0, []string{"SIG"})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	bestPlaced, bestRouted, summary := maybeRetryPlacementRouting(ctx, request, PCBFragmentResult{}, placed, initial, RoutingOptions{Skip: true, Mode: routing.ModeSingleLayer}, request.RoutingRetry)

	assertRetrySummaryInvariant(t, summary, true)
	if summary.StopReason != metadata.ExpectedStopReason {
		t.Fatalf("stop reason = %q, want %q", summary.StopReason, metadata.ExpectedStopReason)
	}
	if summary.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", summary.Attempts)
	}
	if summary.Applied != 1 {
		t.Fatalf("applied = %d, want 1", summary.Applied)
	}
	if !slices.Equal(summary.HintCategories, metadata.ExpectedCategories) {
		t.Fatalf("hint categories = %#v, want %#v", summary.HintCategories, metadata.ExpectedCategories)
	}
	if len(summary.AttemptHistory) != 1 || intFromStageSummary(summary.AttemptHistory[0], "attempt") != 2 {
		t.Fatalf("attempt history = %#v", summary.AttemptHistory)
	}
	if !reflect.DeepEqual(bestRouted.Result, initial.Result) {
		t.Fatalf("best routed was replaced by non-improving attempt: best=%#v initial=%#v", bestRouted.Result, initial.Result)
	}
	if string(bestRouted.Result.Status) != metadata.ExpectedRoutingStatus {
		t.Fatalf("best routing status = %q, want %q", bestRouted.Result.Status, metadata.ExpectedRoutingStatus)
	}
	if diffs := fullBoardRetryConstraintDiff(placed.Request, bestPlaced.Request); len(diffs) != 0 {
		t.Fatalf("hard constraints changed after retry: %#v", diffs)
	}
	if metadata.ExpectedImprovement != "safe_stop_best_preserved" {
		t.Fatalf("metadata = %#v summary=%#v", metadata, summary)
	}
	stageSummary, ok := retrySummaryFromStage(t, bestRouted.Stage)
	if !ok || stageSummary.StopReason != summary.StopReason {
		t.Fatalf("routing stage summary = %#v ok=%v", stageSummary, ok)
	}
}
