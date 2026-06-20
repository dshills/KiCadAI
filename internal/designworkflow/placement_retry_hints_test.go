package designworkflow

import (
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

func TestBuildPlacementRetryHintsMapsRoutingCategories(t *testing.T) {
	tests := []struct {
		name     string
		category routing.RepairCategory
		want     PlacementRetryHintCategory
		eligible bool
	}{
		{name: "route search", category: routing.RepairRouteSearch, want: PlacementRetryIncreaseSpacing, eligible: true},
		{name: "clearance", category: routing.RepairClearance, want: PlacementRetryIncreaseSpacing, eligible: true},
		{name: "length", category: routing.RepairLengthPolicy, want: PlacementRetryReduceDistance, eligible: true},
		{name: "pad", category: routing.RepairPadAccess, want: PlacementRetryImproveFanout, eligible: true},
		{name: "edge", category: routing.RepairBoardBoundary, want: PlacementRetryMoveFromEdge, eligible: true},
		{name: "rules", category: routing.RepairRoutingRules, want: PlacementRetryRelaxRules, eligible: false},
		{name: "zone", category: routing.RepairZonePolicy, want: PlacementRetryUnsupported, eligible: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{{
				Category: tc.category,
				Action:   routing.ActionInspectManually,
				Severity: reports.SeverityError,
				Refs:     []string{"U2", "U1"},
				Nets:     []string{"B", "A"},
			}}, nil)
			if len(hints) != 1 {
				t.Fatalf("hints = %#v", hints)
			}
			if hints[0].Category != tc.want || hints[0].RetryEligible != tc.eligible {
				t.Fatalf("hint = %#v, want category %s eligible %v", hints[0], tc.want, tc.eligible)
			}
			if hints[0].Refs[0] != "U1" || hints[0].Nets[0] != "A" {
				t.Fatalf("refs/nets not sorted: %#v", hints[0])
			}
		})
	}
}

func TestBuildPlacementRetryHintsIncludesPlacementEvidence(t *testing.T) {
	quality := &placement.QualityReport{
		CongestionReports: []placement.CongestionReport{{CellID: "r001_c002", Status: "warning"}},
		FanoutReports:     []placement.FanoutReport{{Ref: "U1", Status: "fail"}},
	}

	hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{
		{Category: routing.RepairRouteSearch, Action: routing.ActionMoveComponents, Severity: reports.SeverityError},
		{Category: routing.RepairPadAccess, Action: routing.ActionFixPadGeometry, Severity: reports.SeverityError, Refs: []string{"U1"}},
	}, quality)

	if len(hints) != 2 {
		t.Fatalf("hints = %#v", hints)
	}
	var foundCongestion, foundFanout bool
	for _, hint := range hints {
		if hint.Category == PlacementRetryIncreaseSpacing && len(hint.PlacementEvidence) == 1 && hint.PlacementEvidence[0] == "congestion:r001_c002:warning:utilization=0.000" {
			foundCongestion = true
		}
		if hint.Category == PlacementRetryImproveFanout && len(hint.PlacementEvidence) == 1 && hint.PlacementEvidence[0] == "fanout:U1:fail" {
			foundFanout = true
		}
	}
	if !foundCongestion || !foundFanout {
		t.Fatalf("missing placement evidence: %#v", hints)
	}
}

func TestBuildPlacementRetryHintsOrdersDeterministically(t *testing.T) {
	hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{
		{Category: routing.RepairPadAccess, Nets: []string{"Z"}},
		{Category: routing.RepairRouteSearch, Nets: []string{"A"}},
		{Category: routing.RepairLengthPolicy, Nets: []string{"B"}},
	}, nil)

	got := []PlacementRetryHintCategory{hints[0].Category, hints[1].Category, hints[2].Category}
	want := []PlacementRetryHintCategory{PlacementRetryImproveFanout, PlacementRetryIncreaseSpacing, PlacementRetryReduceDistance}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("order = %#v, want %#v", got, want)
		}
	}
}

func TestBuildPlacementRetryHintsDeduplicatesIdenticalHints(t *testing.T) {
	diagnostic := routing.RepairDiagnostic{
		Category: routing.RepairRouteSearch,
		Action:   routing.ActionMoveComponents,
		Severity: reports.SeverityError,
		Refs:     []string{"U1"},
		Nets:     []string{"N1"},
	}

	hints := BuildPlacementRetryHints([]routing.RepairDiagnostic{diagnostic, diagnostic}, nil)
	if len(hints) != 1 {
		t.Fatalf("hints = %#v, want one deduplicated hint", hints)
	}
}
