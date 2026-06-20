package designworkflow

import (
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/routing"
)

func TestFilterPlacementRetryHintsHonorsPolicy(t *testing.T) {
	hints := []PlacementRetryHint{
		{Category: PlacementRetryIncreaseSpacing, RetryEligible: true},
		{Category: PlacementRetryImproveFanout, RetryEligible: true},
		{Category: PlacementRetryRelaxRules, RetryEligible: false},
	}
	policy := RoutingRetryPolicySpec{AllowedHintCategories: []PlacementRetryHintCategory{PlacementRetryIncreaseSpacing}}

	filtered := filterPlacementRetryHints(hints, policy)
	if len(filtered) != 1 || filtered[0].Category != PlacementRetryIncreaseSpacing {
		t.Fatalf("filtered hints = %#v", filtered)
	}
}

func TestRoutingAttemptBetterRanksFailures(t *testing.T) {
	current := RoutingStageResult{Result: routing.Result{Status: routing.StatusBlocked}}
	current.Result.Metrics.FailedNetCount = 2
	current.Result.Metrics.RoutedNetCount = 1
	candidate := RoutingStageResult{Result: routing.Result{Status: routing.StatusBlocked}}
	candidate.Result.Metrics.FailedNetCount = 1
	candidate.Result.Metrics.RoutedNetCount = 1

	if !routingAttemptBetter(candidate, current) {
		t.Fatalf("candidate should rank better")
	}
}

func TestRoutingAttemptBetterPrefersRoutedStatus(t *testing.T) {
	current := RoutingStageResult{Result: routing.Result{Status: routing.StatusBlocked}}
	candidate := RoutingStageResult{Result: routing.Result{Status: routing.StatusRouted}}

	if !routingAttemptBetter(candidate, current) {
		t.Fatalf("routed candidate should rank better")
	}
}

func TestPlacementStateHashIgnoresFixedComponents(t *testing.T) {
	placements := []placement.PlacementResult{
		{Ref: "J1", Fixed: true, Position: placement.Placement{XMM: 1, YMM: 1}},
		{Ref: "U1", Position: placement.Placement{XMM: 2.001, YMM: 3.001, RotationDeg: 90, Layer: "F.Cu"}},
	}
	changedFixed := []placement.PlacementResult{
		{Ref: "J1", Fixed: true, Position: placement.Placement{XMM: 9, YMM: 9}},
		{Ref: "U1", Position: placement.Placement{XMM: 2.001, YMM: 3.001, RotationDeg: 90, Layer: "F.Cu"}},
	}

	if placementStateHash(placements) != placementStateHash(changedFixed) {
		t.Fatalf("fixed placement affected state hash")
	}
}
