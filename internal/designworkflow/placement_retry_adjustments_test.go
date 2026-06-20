package designworkflow

import (
	"testing"

	"kicadai/internal/placement"
)

func TestBuildPlacementRetryAdjustmentIncreasesSpacing(t *testing.T) {
	req := retryPlacementRequest()
	adjusted, adjustment := BuildPlacementRetryAdjustment(req, []PlacementRetryHint{{
		Category:      PlacementRetryIncreaseSpacing,
		RetryEligible: true,
	}}, 1)

	if !adjustment.Applied || adjustment.SpacingDeltaMM != 1 {
		t.Fatalf("adjustment = %#v", adjustment)
	}
	if adjusted.Rules.ComponentSpacingMM != req.Rules.ComponentSpacingMM+1 {
		t.Fatalf("component spacing = %.2f", adjusted.Rules.ComponentSpacingMM)
	}
}

func TestBuildPlacementRetryAdjustmentAddsReduceDistanceRule(t *testing.T) {
	req := retryPlacementRequest()
	adjusted, adjustment := BuildPlacementRetryAdjustment(req, []PlacementRetryHint{{
		Category:      PlacementRetryReduceDistance,
		RetryEligible: true,
		Nets:          []string{"N1"},
	}}, 2)

	if !adjustment.Applied || len(adjustment.ProximityRules) != 1 {
		t.Fatalf("adjustment = %#v", adjustment)
	}
	if len(adjusted.ProximityRules) != 1 || adjusted.ProximityRules[0].AnchorRef != "C1" || adjusted.ProximityRules[0].TargetRefs[0] != "R1" {
		t.Fatalf("proximity rules = %#v", adjusted.ProximityRules)
	}
}

func TestBuildPlacementRetryAdjustmentPreservesFixedComponents(t *testing.T) {
	req := retryPlacementRequest()
	req.Components[0].Fixed = true
	req.Components[0].Position = &placement.Placement{XMM: 1, YMM: 2}

	adjusted, _ := BuildPlacementRetryAdjustment(req, []PlacementRetryHint{{
		Category:      PlacementRetryIncreaseSpacing,
		RetryEligible: true,
	}}, 1)

	if !adjusted.Components[0].Fixed || adjusted.Components[0].Position == nil || adjusted.Components[0].Position.XMM != 1 {
		t.Fatalf("fixed component mutated: %#v", adjusted.Components[0])
	}
}

func TestBuildPlacementRetryAdjustmentSkipsIneligibleHints(t *testing.T) {
	req := retryPlacementRequest()
	adjusted, adjustment := BuildPlacementRetryAdjustment(req, []PlacementRetryHint{{
		Category:      PlacementRetryRelaxRules,
		RetryEligible: false,
	}}, 1)

	if adjustment.Applied || len(adjustment.SkippedReasons) != 1 {
		t.Fatalf("adjustment = %#v", adjustment)
	}
	if adjusted.Rules.ComponentSpacingMM != req.Rules.ComponentSpacingMM {
		t.Fatalf("spacing changed for ineligible hint")
	}
}

func TestBuildPlacementRetryAdjustmentIsDeterministic(t *testing.T) {
	req := retryPlacementRequest()
	hints := []PlacementRetryHint{
		{Category: PlacementRetryReduceDistance, RetryEligible: true, Nets: []string{"N2"}},
		{Category: PlacementRetryReduceDistance, RetryEligible: true, Nets: []string{"N1"}},
	}
	req.Nets = append(req.Nets, placement.Net{Name: "N2", Endpoints: []placement.Endpoint{{Ref: "U1", Pin: "1"}, {Ref: "R1", Pin: "2"}}})

	_, adjustment := BuildPlacementRetryAdjustment(req, hints, 1)
	if len(adjustment.ProximityRules) != 2 || adjustment.ProximityRules[0] != "retry_reduce_distance:N1:C1:R1" || adjustment.ProximityRules[1] != "retry_reduce_distance:N2:U1:R1" {
		t.Fatalf("proximity rule order = %#v", adjustment.ProximityRules)
	}
}

func retryPlacementRequest() placement.Request {
	return placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: 40, HeightMM: 30},
		Components: []placement.Component{
			{Ref: "R1", Bounds: placement.Bounds{WidthMM: 2, HeightMM: 1, Source: placement.BoundsExplicit}},
			{Ref: "C1", Bounds: placement.Bounds{WidthMM: 2, HeightMM: 1, Source: placement.BoundsExplicit}},
			{Ref: "U1", Bounds: placement.Bounds{WidthMM: 4, HeightMM: 4, Source: placement.BoundsExplicit}},
		},
		Nets: []placement.Net{{Name: "N1", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "C1", Pin: "1"}}}},
		Rules: placement.Rules{
			ComponentSpacingMM: 1,
			GroupSpacingMM:     1,
		},
	}
}
