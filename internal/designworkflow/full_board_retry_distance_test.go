package designworkflow

import (
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestFullBoardRetryDistanceAddsProximityRules(t *testing.T) {
	metadata := loadFullBoardRetryMetadata(t, "distance_rules")
	placed := fullBoardRetrySeedPlacement(t, "distance_rules")
	hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{{
		Category: routing.RepairLengthPolicy,
		Action:   routing.ActionRelaxLengthPolicy,
		Severity: reports.SeverityError,
		Nets:     []string{"SIG"},
	}}, nil)

	if len(hints) != 1 || hints[0].Category != PlacementRetryReduceDistance || !hints[0].RetryEligible {
		t.Fatalf("hints = %#v", hints)
	}
	adjusted, adjustment := BuildPlacementRetryAdjustment(placed.Request, hints, 1)
	if !adjustment.Applied || !slices.Contains(adjustment.ProximityRules, "retry_reduce_distance:SIG:U1:R1") {
		t.Fatalf("adjustment = %#v", adjustment)
	}
	ruleIDs := placementRetryProximityRuleIDs(adjusted.ProximityRules)
	wantRules := []string{
		"retry_reduce_distance:SIG:U1:D1",
		"retry_reduce_distance:SIG:U1:R1",
	}
	for _, want := range wantRules {
		if _, ok := ruleIDs[want]; !ok {
			t.Fatalf("missing proximity rule %q in %#v", want, adjusted.ProximityRules)
		}
	}
	adjustedAgain, duplicate := BuildPlacementRetryAdjustment(adjusted, []PlacementRetryHint{{
		Category:      PlacementRetryReduceDistance,
		RetryEligible: true,
		Nets:          []string{"SIG"},
	}}, 2)
	if duplicate.Applied || len(duplicate.ProximityRules) != 0 || len(adjustedAgain.ProximityRules) != len(adjusted.ProximityRules) {
		t.Fatalf("duplicate reduce-distance adjustment = %#v adjusted=%#v", duplicate, adjustedAgain.ProximityRules)
	}
	if diffs := fullBoardRetryConstraintDiff(placed.Request, adjusted); len(diffs) != 0 {
		t.Fatalf("hard constraints changed after retry: %#v", diffs)
	}
	if metadata.ExpectedImprovement != "proximity_rule_evidence" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
