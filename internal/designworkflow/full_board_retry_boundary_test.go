package designworkflow

import (
	"context"
	"slices"
	"testing"
	"time"

	"kicadai/internal/routing"
)

func TestFullBoardRetrySafeStopEvidenceContract(t *testing.T) {
	const fixture = "safe_stop"
	metadata := loadFullBoardRetryMetadata(t, fixture)
	request := loadFullBoardRetryRequestForTest(t, fixture)
	placed := fullBoardRetrySeedPlacement(t, fixture)
	initial := fullBoardRetryBlockedRouteSearchResult(0, []string{"SIG"})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	bestPlaced, bestRouted, summary := maybeRetryPlacementRouting(ctx, request, PCBFragmentResult{}, placed, initial, RoutingOptions{Skip: true, Mode: routing.ModeSingleLayer}, request.RoutingRetry)
	if err := ctx.Err(); err != nil {
		t.Fatalf("retry context ended early: %v", err)
	}
	if summary.Attempts == 0 {
		t.Fatalf("retry summary was not populated: %#v", summary)
	}
	stageSummary, ok := retrySummaryFromStage(t, bestRouted.Stage)
	if !ok {
		t.Fatalf("safe-stop routing stage missing retry summary: %#v", bestRouted.Stage.Summary)
	}
	if summary.StopReason != metadata.ExpectedStopReason || stageSummary.StopReason != metadata.ExpectedStopReason {
		t.Fatalf("safe-stop retry evidence = %#v summary=%#v metadata=%#v", stageSummary, summary, metadata)
	}
	if stageSummary.Applied < metadata.ExpectedMinApplied || stageSummary.Attempts < metadata.ExpectedMinAttempts {
		t.Fatalf("safe-stop retry counts = %#v metadata=%#v", stageSummary, metadata)
	}
	if bestRouted.Result.Status != routing.Status(metadata.ExpectedFinalStatus) {
		t.Fatalf("safe-stop final status = %s, want %s", bestRouted.Result.Status, metadata.ExpectedFinalStatus)
	}
	gotCategories := slices.Clone(stageSummary.HintCategories)
	wantCategories := slices.Clone(metadata.ExpectedCategories)
	slices.Sort(gotCategories)
	slices.Sort(wantCategories)
	if !slices.Equal(gotCategories, wantCategories) {
		t.Fatalf("safe-stop hint categories = %#v, want %#v", gotCategories, wantCategories)
	}
	if routingAttemptBetter(bestRouted, initial) {
		t.Fatalf("safe-stop unexpectedly improved: best=%#v initial=%#v", bestRouted.Result.Metrics, initial.Result.Metrics)
	}
	if diffs := fullBoardRetryConstraintDiff(placed.Request, bestPlaced.Request); len(diffs) != 0 {
		t.Fatalf("hard constraints changed after safe-stop retry: %#v", diffs)
	}
}

func TestFullBoardRetryGeneratedUnsupportedBoundaryEvidence(t *testing.T) {
	const fixture = "generated_led_connectivity"
	metadata := loadFullBoardRetryMetadata(t, fixture)
	result := runFullBoardRetryFixture(t, fixture)
	evidence := fullBoardRetryEvidenceFromWorkflow(t, fixture, result)

	if metadata.FixtureClass != "generated_connectivity_boundary" {
		t.Fatalf("metadata fixture class = %q", metadata.FixtureClass)
	}
	if !evidence.HasPadHydration || evidence.PadHydration.HydratedComponents < metadata.ExpectedMinHydratedPads {
		t.Fatalf("pad hydration evidence = %#v metadata=%#v", evidence.PadHydration, metadata)
	}
	if !evidence.HasRetry || evidence.Retry.StopReason != metadata.ExpectedStopReason {
		t.Fatalf("generated boundary retry evidence = %#v metadata=%#v", evidence.Retry, metadata)
	}
	if evidence.Retry.Applied < metadata.ExpectedMinApplied || evidence.Retry.Attempts < metadata.ExpectedMinAttempts {
		t.Fatalf("generated boundary retry counts = %#v metadata=%#v", evidence.Retry, metadata)
	}
	if len(evidence.Retry.HintCategories) != 0 {
		t.Fatalf("generated boundary should not expose eligible hint categories: %#v", evidence.Retry.HintCategories)
	}
	if evidence.RoutingStatus != routing.Status(metadata.ExpectedRoutingStatus) {
		t.Fatalf("generated boundary routing status = %s, want %s", evidence.RoutingStatus, metadata.ExpectedRoutingStatus)
	}
}
