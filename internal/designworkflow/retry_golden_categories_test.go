package designworkflow

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestRetryGoldenIncreaseSpacingFixture(t *testing.T) {
	request := loadRetryFixtureRequest(t, "increase_spacing")
	if !request.RoutingRetry.Enabled || len(request.RoutingRetry.AllowedHintCategories) != 1 || request.RoutingRetry.AllowedHintCategories[0] != PlacementRetryIncreaseSpacing {
		t.Fatalf("retry policy = %#v", request.RoutingRetry)
	}

	hints := filterPlacementRetryHints(BuildPlacementRetryHints([]routing.RepairDiagnostic{{
		Category: routing.RepairRouteSearch,
		Action:   routing.ActionMoveComponents,
		Severity: reports.SeverityError,
		Nets:     []string{"VIN"},
	}}, &placement.QualityReport{
		CongestionReports: []placement.CongestionReport{{
			CellID:      "r001_c001",
			Status:      "warning",
			Utilization: 1.25,
		}},
	}), request.RoutingRetry)
	if len(hints) != 1 || hints[0].Category != PlacementRetryIncreaseSpacing {
		t.Fatalf("hints = %#v", hints)
	}
	if !retryGoldenHasEvidencePrefix(hints[0].PlacementEvidence, "congestion:r001_c001:warning:utilization=") {
		t.Fatalf("missing congestion evidence: %#v", hints[0].PlacementEvidence)
	}

	req := retryGoldenPlacementRequest()
	adjusted, adjustment := BuildPlacementRetryAdjustment(req, hints, 1)
	if !adjustment.Applied || math.Abs(adjustment.SpacingDeltaMM-1) > 1e-9 || len(adjustment.ProximityRules) != 0 {
		t.Fatalf("adjustment = %#v", adjustment)
	}
	assertRetryGoldenPlacementConstraintsPreserved(t, req, adjusted)
	assertRetryGoldenSpacingDelta(t, req, adjusted, 1)
}

func TestRetryGoldenImproveFanoutFixture(t *testing.T) {
	request := loadRetryFixtureRequest(t, "improve_fanout")
	if !request.RoutingRetry.Enabled || len(request.RoutingRetry.AllowedHintCategories) != 1 || request.RoutingRetry.AllowedHintCategories[0] != PlacementRetryImproveFanout {
		t.Fatalf("retry policy = %#v", request.RoutingRetry)
	}

	hints := filterPlacementRetryHints(BuildPlacementRetryHints([]routing.RepairDiagnostic{{
		Category: routing.RepairPadAccess,
		Action:   routing.ActionFixPadGeometry,
		Severity: reports.SeverityError,
		Refs:     []string{"U1"},
		Nets:     []string{"SDA"},
	}}, &placement.QualityReport{
		FanoutReports: []placement.FanoutReport{{
			Ref:    "U1",
			Status: "fail",
		}},
	}), request.RoutingRetry)
	if len(hints) != 1 || hints[0].Category != PlacementRetryImproveFanout {
		t.Fatalf("hints = %#v", hints)
	}
	if !retryGoldenHasEvidencePrefix(hints[0].PlacementEvidence, "fanout:U1:fail") {
		t.Fatalf("missing fanout evidence: %#v", hints[0].PlacementEvidence)
	}

	req := retryGoldenPlacementRequest()
	adjusted, adjustment := BuildPlacementRetryAdjustment(req, hints, 1)
	if !adjustment.Applied || math.Abs(adjustment.SpacingDeltaMM-1) > 1e-9 || len(adjustment.ProximityRules) != 0 {
		t.Fatalf("adjustment = %#v", adjustment)
	}
	assertRetryGoldenPlacementConstraintsPreserved(t, req, adjusted)
	assertRetryGoldenSpacingDelta(t, req, adjusted, 1)
}

func TestRetryGoldenReduceDistanceFixture(t *testing.T) {
	request := loadRetryFixtureRequest(t, "reduce_distance")
	if !request.RoutingRetry.Enabled || len(request.RoutingRetry.AllowedHintCategories) != 1 || request.RoutingRetry.AllowedHintCategories[0] != PlacementRetryReduceDistance {
		t.Fatalf("retry policy = %#v", request.RoutingRetry)
	}

	hints := filterPlacementRetryHints(BuildPlacementRetryHints([]routing.RepairDiagnostic{{
		Category: routing.RepairLengthPolicy,
		Action:   routing.ActionRelaxLengthPolicy,
		Severity: reports.SeverityError,
		Nets:     []string{"SDA"},
	}}, nil), request.RoutingRetry)
	if len(hints) != 1 || hints[0].Category != PlacementRetryReduceDistance {
		t.Fatalf("hints = %#v", hints)
	}

	req := retryGoldenPlacementRequest()
	adjusted, adjustment := BuildPlacementRetryAdjustment(req, hints, 1)
	wantRules := []string{
		"retry_reduce_distance:SDA:U1:R1",
	}
	if !adjustment.Applied || !retryGoldenSameStrings(adjustment.ProximityRules, wantRules) {
		t.Fatalf("adjustment = %#v, want rules %#v", adjustment, wantRules)
	}
	rulesByID := retryGoldenProximityRulesByID(&adjusted)
	if len(rulesByID) != 1 {
		t.Fatalf("proximity rules = %#v", adjusted.ProximityRules)
	}
	for _, wantID := range wantRules {
		rule, ok := rulesByID[wantID]
		if !ok {
			t.Fatalf("missing proximity rule %s in %#v", wantID, adjusted.ProximityRules)
		}
		if rule.AnchorRef != "U1" || rule.MaxDistanceMM != placementRetryMaxProximityMM || rule.Source != "routing_retry" {
			t.Fatalf("rule = %#v, want id %s anchored at U1", rule, wantID)
		}
	}

	adjustedAgain, duplicate := BuildPlacementRetryAdjustment(adjusted, hints, 2)
	if duplicate.Applied || len(duplicate.ProximityRules) != 0 || len(adjustedAgain.ProximityRules) != len(adjusted.ProximityRules) {
		t.Fatalf("duplicate adjustment = %#v adjusted=%#v", duplicate, adjustedAgain.ProximityRules)
	}
}

func TestRetryGoldenUnsupportedZoneSkipsPlacementMutation(t *testing.T) {
	request := loadRetryFixtureRequest(t, "unsupported_zone")
	if !request.RoutingRetry.Enabled || len(request.RoutingRetry.AllowedHintCategories) != 1 || request.RoutingRetry.AllowedHintCategories[0] != PlacementRetryUnsupported {
		t.Fatalf("retry policy = %#v", request.RoutingRetry)
	}
	hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{{
		Category: routing.RepairZonePolicy,
		Action:   routing.ActionResolveZonePolicy,
		Severity: reports.SeverityError,
		Nets:     []string{"GND"},
	}}, nil)
	if len(hints) != 1 || hints[0].Category != PlacementRetryUnsupported || hints[0].RetryEligible {
		t.Fatalf("hints = %#v", hints)
	}
	filtered := filterPlacementRetryHints(hints, request.RoutingRetry)
	if len(filtered) != 0 {
		t.Fatalf("unsupported hints should not pass retry filter: %#v", filtered)
	}
	assertRetryGoldenNoPlacementMutation(t, hints)
}

func TestRetryGoldenRelaxRulesSkipsPlacementMutation(t *testing.T) {
	request := loadRetryFixtureRequest(t, "relax_rules")
	if !request.RoutingRetry.Enabled || len(request.RoutingRetry.AllowedHintCategories) != 1 || request.RoutingRetry.AllowedHintCategories[0] != PlacementRetryRelaxRules {
		t.Fatalf("retry policy = %#v", request.RoutingRetry)
	}
	hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{{
		Category: routing.RepairRoutingRules,
		Action:   routing.ActionAdjustRoutingRules,
		Severity: reports.SeverityError,
		Nets:     []string{"VIN"},
	}}, nil)
	if len(hints) != 1 || hints[0].Category != PlacementRetryRelaxRules || hints[0].RetryEligible {
		t.Fatalf("hints = %#v", hints)
	}
	filtered := filterPlacementRetryHints(hints, request.RoutingRetry)
	if len(filtered) != 0 {
		t.Fatalf("rule-only hints should not pass retry filter: %#v", filtered)
	}
	assertRetryGoldenNoPlacementMutation(t, hints)
}

func retryGoldenPlacementRequest() placement.Request {
	return placement.Request{
		Board: placement.BoardPlacementArea{WidthMM: 40, HeightMM: 30, MarginMM: 2},
		Components: []placement.Component{
			{
				Ref:      "J1",
				Bounds:   placement.Bounds{WidthMM: 8, HeightMM: 4, Source: placement.BoundsExplicit},
				Fixed:    true,
				Position: &placement.Placement{XMM: 3, YMM: 15, RotationDeg: 0, Layer: "F.Cu"},
				Edge:     placement.EdgeLeft,
			},
			{Ref: "U1", Bounds: placement.Bounds{WidthMM: 6, HeightMM: 6, Source: placement.BoundsExplicit}},
			{Ref: "R1", Bounds: placement.Bounds{WidthMM: 3, HeightMM: 1.5, Source: placement.BoundsExplicit}},
		},
		Nets: []placement.Net{
			{Name: "VIN", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U1", Pin: "1"}}},
			{Name: "SDA", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "2"}, {Ref: "U1", Pin: "2"}, {Ref: "R1", Pin: "1"}}},
		},
		Keepouts: []placement.Keepout{{
			ID:     "mounting_hole_keepout",
			Bounds: placement.Rect{Min: placement.Point{XMM: 18, YMM: 12}, Max: placement.Point{XMM: 22, YMM: 16}},
			Layers: []string{"F.Cu", "B.Cu"},
			Reason: "mechanical mounting hole",
		}},
		Rules: placement.Rules{
			ComponentSpacingMM: 1,
			GroupSpacingMM:     1,
		},
	}
}

func assertRetryGoldenNoPlacementMutation(t *testing.T, hints []PlacementRetryHint) {
	t.Helper()
	req := placement.NormalizeRequest(retryGoldenPlacementRequest())
	before := placement.CloneRequest(req)
	adjusted, adjustment := BuildPlacementRetryAdjustment(req, hints, 1)
	if adjustment.Applied {
		t.Fatalf("ineligible adjustment applied: %#v", adjustment)
	}
	assertRetryGoldenEqual(t, "components", before.Components, adjusted.Components)
	assertRetryGoldenEqual(t, "nets", before.Nets, adjusted.Nets)
	assertRetryGoldenEqual(t, "keepouts", before.Keepouts, adjusted.Keepouts)
	assertRetryGoldenEqual(t, "proximity_rules", before.ProximityRules, adjusted.ProximityRules)
	assertRetryGoldenEqual(t, "board", before.Board, adjusted.Board)
	assertRetryGoldenEqual(t, "existing", before.Existing, adjusted.Existing)
	assertRetryGoldenEqual(t, "seed", before.Seed, adjusted.Seed)
	assertRetryGoldenEqual(t, "rules", before.Rules, adjusted.Rules)
}

func assertRetryGoldenFixedComponentPreserved(t *testing.T, before, after placement.Request, ref string) {
	t.Helper()
	beforeComponent := retryGoldenComponentByRef(&before, ref)
	afterComponent := retryGoldenComponentByRef(&after, ref)
	if beforeComponent == nil || afterComponent == nil {
		t.Fatalf("missing fixed ref %s before=%#v after=%#v", ref, before.Components, after.Components)
	}
	if !afterComponent.Fixed || !reflect.DeepEqual(afterComponent.Position, beforeComponent.Position) {
		t.Fatalf("fixed ref %s changed: before=%#v after=%#v", ref, beforeComponent, afterComponent)
	}
}

func assertRetryGoldenPlacementConstraintsPreserved(t *testing.T, before, after placement.Request) {
	t.Helper()
	assertRetryGoldenFixedComponentPreserved(t, before, after, "J1")
	if !reflect.DeepEqual(after.Keepouts, before.Keepouts) {
		t.Fatalf("keepouts not preserved: %#v", after.Keepouts)
	}
	beforeJ1 := retryGoldenComponentByRef(&before, "J1")
	afterJ1 := retryGoldenComponentByRef(&after, "J1")
	if beforeJ1 == nil || afterJ1 == nil || afterJ1.Edge != beforeJ1.Edge {
		t.Fatalf("edge constraint changed: before=%#v after=%#v", beforeJ1, afterJ1)
	}
}

func assertRetryGoldenSpacingDelta(t *testing.T, before, after placement.Request, delta float64) {
	t.Helper()
	if math.Abs(after.Rules.ComponentSpacingMM-(before.Rules.ComponentSpacingMM+delta)) > 1e-9 ||
		math.Abs(after.Rules.GroupSpacingMM-(before.Rules.GroupSpacingMM+delta)) > 1e-9 {
		t.Fatalf("spacing not adjusted by %.2f: before=%#v after=%#v", delta, before.Rules, after.Rules)
	}
}

func retryGoldenHasEvidencePrefix(evidence []string, prefix string) bool {
	return slices.ContainsFunc(evidence, func(item string) bool {
		return strings.HasPrefix(item, prefix)
	})
}

func retryGoldenComponentByRef(request *placement.Request, ref string) *placement.Component {
	for index := range request.Components {
		if request.Components[index].Ref == ref {
			return &request.Components[index]
		}
	}
	return nil
}

func retryGoldenSameStrings(got, want []string) bool {
	gotSorted := slices.Clone(got)
	wantSorted := slices.Clone(want)
	slices.Sort(gotSorted)
	slices.Sort(wantSorted)
	return slices.Equal(gotSorted, wantSorted)
}

func retryGoldenProximityRulesByID(request *placement.Request) map[string]placement.ProximityRule {
	rules := make(map[string]placement.ProximityRule, len(request.ProximityRules))
	for _, rule := range request.ProximityRules {
		rules[rule.ID] = rule
	}
	return rules
}

func assertRetryGoldenEqual(t *testing.T, label string, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s changed: want=%#v got=%#v", label, want, got)
	}
}
