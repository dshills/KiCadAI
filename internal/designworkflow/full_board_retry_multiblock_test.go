package designworkflow

import (
	"slices"
	"testing"

	"kicadai/internal/placement"
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
	if metadata.FixtureClass != "generated_multiblock_mobility_boundary" {
		t.Fatalf("fixture class = %q", metadata.FixtureClass)
	}
	if padSummary.PadCount < metadata.ExpectedMinHydratedPads || padSummary.MissingComponents != 0 {
		t.Fatalf("pad hydration = %#v metadata=%#v", padSummary, metadata)
	}
	if placementStage.Status != StageStatusOK {
		t.Fatalf("placement status = %s, want ok; issues=%#v", placementStage.Status, placementStage.Issues)
	}
	mobility, ok := placementStage.Summary["mobility"].(placement.MobilitySummary)
	if !ok {
		t.Fatalf("mobility summary missing: %#v", placementStage.Summary)
	}
	if mobility.Total == 0 || mobility.FixedCount+mobility.GroupTransformCount != mobility.Total {
		t.Fatalf("expected generated multiblock boundary mobility evidence: %#v", mobility)
	}
	localRoutes, ok := fullBoardRetryLocalRouteMobilitySummary(routingStage)
	if !ok || localRoutes.Total == 0 || localRoutes.Transformable+localRoutes.Rebuildable+localRoutes.Preserved == 0 {
		t.Fatalf("expected generated multiblock local-route mobility evidence: %#v", routingStage.Summary)
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
