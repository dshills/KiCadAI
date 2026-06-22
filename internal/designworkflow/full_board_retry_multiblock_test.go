package designworkflow

import (
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestFullBoardRetryGeneratedMultiBlockBoundaryEvidence(t *testing.T) {
	const fixture = "generated_sensor_multiblock"
	metadata := loadFullBoardRetryMetadata(t, fixture)
	result := runFullBoardRetryFixture(t, fixture)
	placementStage, ok := stageByName(result, StagePlacement)
	if !ok {
		t.Fatalf("placement stage missing: got stages %#v", stageNamesForRetryTest(result.Stages))
	}
	routingStage, ok := stageByName(result, StageRouting)
	if !ok {
		t.Fatalf("routing stage missing: got stages %#v", stageNamesForRetryTest(result.Stages))
	}
	padSummary, ok := fullBoardRetryPadHydrationSummary(placementStage)
	if !ok {
		t.Fatalf("pad hydration summary missing: %#v", placementStage.Summary)
	}
	if metadata.FixtureClass != "generated_multiblock_boundary" {
		t.Fatalf("fixture class = %q", metadata.FixtureClass)
	}
	if padSummary.PadCount < metadata.ExpectedMinHydratedPads || padSummary.MissingComponents != 0 {
		t.Fatalf("pad hydration = %#v metadata=%#v", padSummary, metadata)
	}
	if placementStage.Status != StageStatusBlocked {
		t.Fatalf("placement status = %s, want blocked", placementStage.Status)
	}
	if !hasIssueCode(placementStage.Issues, reports.CodePlacementOutsideBoard) {
		t.Fatalf("placement issues did not document fixed placement boundary: %#v", placementStage.Issues)
	}
	if got := string(routingStage.Status); got != metadata.ExpectedRoutingStatus {
		t.Fatalf("routing status = %s, want %s", got, metadata.ExpectedRoutingStatus)
	}
}

func TestFullBoardRetryConstraintPreservationEvidenceContract(t *testing.T) {
	tests := []struct {
		fixture string
		net     string
	}{
		{fixture: "distance_rules", net: "SIG"},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			metadata := loadFullBoardRetryMetadata(t, tc.fixture)
			placed := fullBoardRetrySeedPlacement(t, tc.fixture)
			hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{{
				Category: routing.RepairLengthPolicy,
				Nets:     []string{tc.net},
			}}, nil)
			adjusted, adjustment := BuildPlacementRetryAdjustment(placed.Request, hints, 1)
			if !adjustment.Applied {
				t.Fatalf("retry adjustment was not applied: %#v", adjustment)
			}
			diffs := fullBoardRetryConstraintDiff(placed.Request, adjusted)
			for _, preserved := range metadata.PreserveConstraints {
				if slices.Contains(diffs, preserved) {
					t.Fatalf("fixture %s changed preserved constraint %q: diffs=%#v", tc.fixture, preserved, diffs)
				}
			}
		})
	}
}

func stageNamesForRetryTest(stages []StageResult) []string {
	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, string(stage.Name))
	}
	return names
}
