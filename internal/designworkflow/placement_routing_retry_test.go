package designworkflow

import (
	"encoding/json"
	"math"
	"testing"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
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

func TestPlacementRoutingRetryAttemptSummaryNormalizesEvidence(t *testing.T) {
	summary := normalizePlacementRoutingRetryAttempt(placementRoutingRetryAttemptSummary{
		Attempt:            -1,
		RouteScore:         math.Inf(1),
		BaselineRouteScore: math.NaN(),
		PlacementScore:     math.Inf(-1),
		SkippedNets:        -4,
	})
	if summary.Attempt != 0 || summary.RouteScore != 0 || summary.BaselineRouteScore != 0 || summary.PlacementScore != 0 || summary.SkippedNets != 0 {
		t.Fatalf("normalized summary = %#v", summary)
	}
	if summary.DRCStatus != retryEvidenceSkipped || summary.DRCSource != "skipped" {
		t.Fatalf("DRC normalization = %#v", summary)
	}
}

func TestPlacementRoutingRetryAttemptSummaryCountsIssues(t *testing.T) {
	routed := RoutingStageResult{
		Result: routing.Result{Status: routing.StatusPartial},
		Stage: StageResult{Issues: []reports.Issue{
			{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning},
			{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked},
		}},
	}
	routed.Result.Metrics.RoutedNetCount = 1
	routed.Result.Metrics.FailedNetCount = 2

	summary := placementRoutingAttemptSummaryForResult(2, nil, nil, routed, "")
	if summary.Attempt != 2 || summary.RoutingStatus != routing.StatusPartial || summary.RoutedNets != 1 || summary.FailedNets != 2 {
		t.Fatalf("route summary = %#v", summary)
	}
	if summary.BoardValidationIssueCount != 2 || summary.BoardValidationBlocking != 1 {
		t.Fatalf("issue summary = %#v", summary)
	}
}

func TestPlacementRoutingRetrySummaryJSONKeepsExistingFields(t *testing.T) {
	summary := placementRoutingRetrySummary{
		Enabled:         true,
		Attempts:        2,
		Applied:         1,
		StopReason:      "max_attempts",
		SelectedAttempt: 2,
		SelectedReason:  "more_routed_nets",
		AttemptHistory: []placementRoutingRetryAttemptSummary{{
			Attempt:       2,
			RoutingStatus: routing.StatusRouted,
			RoutedNets:    2,
			Selected:      true,
		}},
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if decoded["enabled"] != true || decoded["stop_reason"] != "max_attempts" || int(decoded["selected_attempt"].(float64)) != 2 {
		t.Fatalf("decoded summary = %#v", decoded)
	}
	history := decoded["attempt_history"].([]any)
	first := history[0].(map[string]any)
	if first["routing_status"] != string(routing.StatusRouted) || first["selected"] != true {
		t.Fatalf("decoded attempt = %#v", first)
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
