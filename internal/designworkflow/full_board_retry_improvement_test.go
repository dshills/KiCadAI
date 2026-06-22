package designworkflow

import (
	"context"
	"slices"
	"testing"
	"time"

	"kicadai/internal/routing"
)

func TestFullBoardRetryImprovementEvidenceContract(t *testing.T) {
	const fixture = "spacing_improves"
	metadata := loadFullBoardRetryMetadata(t, fixture)
	request := loadFullBoardRetryRequestForTest(t, fixture)
	placed := fullBoardRetrySeedPlacement(t, fixture)
	const initialFailedNetCount = 2
	initial := fullBoardRetryBlockedRouteSearchResult(initialFailedNetCount, []string{"SIG"})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	bestPlaced, bestRouted, summary := maybeRetryPlacementRouting(ctx, request, PCBFragmentResult{}, placed, initial, RoutingOptions{Mode: routing.ModeSingleLayer}, request.RoutingRetry)
	if err := ctx.Err(); err != nil {
		t.Fatalf("retry context ended early: %v", err)
	}
	if summary.Attempts == 0 {
		t.Fatalf("retry summary was not populated: %#v", summary)
	}
	result := WorkflowResult{Stages: []StageResult{placed.Stage, bestRouted.Stage}}
	evidence := fullBoardRetryEvidenceFromWorkflow(t, fixture, result)

	if metadata.FixtureClass != "seed_improvement" || metadata.ExpectedImprovedMetric != "routing_status_rank" {
		t.Fatalf("metadata does not declare improvement contract: %#v", metadata)
	}
	if !evidence.HasRetry || !summary.Enabled {
		t.Fatalf("retry evidence = %#v summary=%#v metadata=%#v", evidence, summary, metadata)
	}
	retry := evidence.Retry
	if retry.Applied < metadata.ExpectedMinApplied || retry.Attempts < metadata.ExpectedMinAttempts {
		t.Fatalf("retry counts = %#v metadata=%#v", retry, metadata)
	}
	if retry.StopReason != metadata.ExpectedStopReason {
		t.Fatalf("stop reason = %q, want %q", retry.StopReason, metadata.ExpectedStopReason)
	}
	if string(evidence.RoutingStatus) != metadata.ExpectedRoutingStatus {
		t.Fatalf("routing status = %s, want %s", evidence.RoutingStatus, metadata.ExpectedRoutingStatus)
	}
	if evidence.BaselineRoutingStatus != routing.Status(metadata.ExpectedBaselineStatus) || evidence.FinalRoutingStatus != routing.Status(metadata.ExpectedFinalStatus) {
		t.Fatalf("routing status delta = %s -> %s, want %s -> %s", evidence.BaselineRoutingStatus, evidence.FinalRoutingStatus, metadata.ExpectedBaselineStatus, metadata.ExpectedFinalStatus)
	}
	if routingStatusRank(evidence.FinalRoutingStatus) <= routingStatusRank(evidence.BaselineRoutingStatus) {
		t.Fatalf("routing status did not improve: baseline=%s final=%s", evidence.BaselineRoutingStatus, evidence.FinalRoutingStatus)
	}
	for _, category := range metadata.ExpectedCategories {
		if !slices.Contains(retry.HintCategories, category) {
			t.Fatalf("hint categories = %#v, missing %q", retry.HintCategories, category)
		}
	}
	if len(retry.AttemptHistory) == 0 {
		t.Fatalf("attempt history missing: %#v", retry)
	}
	if diffs := fullBoardRetryConstraintDiff(placed.Request, bestPlaced.Request); len(diffs) != 0 {
		t.Fatalf("hard constraints changed after retry: %#v", diffs)
	}
}
