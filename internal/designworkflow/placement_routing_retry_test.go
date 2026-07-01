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

func TestPlacementRoutingAttemptRankingPrefersValidationBlockersOverRoutedNets(t *testing.T) {
	current := placementRoutingRetryAttemptSummary{
		Attempt:                 1,
		RoutingStatus:           routing.StatusPartial,
		RoutedNets:              2,
		FailedNets:              1,
		BoardValidationBlocking: 0,
	}
	candidate := placementRoutingRetryAttemptSummary{
		Attempt:                 2,
		RoutingStatus:           routing.StatusPartial,
		RoutedNets:              4,
		FailedNets:              0,
		BoardValidationBlocking: 1,
	}

	if placementRoutingAttemptBetter(candidate, current, RoutingRetryPolicySpec{}) {
		t.Fatalf("candidate with validation blocker should not outrank current")
	}
}

func TestPlacementRoutingAttemptRankingRequiresCleanDRCWhenRequired(t *testing.T) {
	current := placementRoutingRetryAttemptSummary{
		Attempt:          1,
		RoutingStatus:    routing.StatusPartial,
		RoutedNets:       1,
		DRCStatus:        retryEvidencePass,
		DRCBlockingCount: 0,
	}
	candidate := placementRoutingRetryAttemptSummary{
		Attempt:          2,
		RoutingStatus:    routing.StatusRouted,
		RoutedNets:       3,
		DRCStatus:        retryEvidenceFail,
		DRCBlockingCount: 1,
	}

	if placementRoutingAttemptBetter(candidate, current, RoutingRetryPolicySpec{DRCPolicy: RetryDRCPolicyRequired}) {
		t.Fatalf("candidate with required DRC failure should not outrank clean current")
	}
}

func TestPlacementRoutingAttemptRankingUsesScoresAndAttemptTieBreak(t *testing.T) {
	current := placementRoutingRetryAttemptSummary{Attempt: 1, RoutingStatus: routing.StatusPartial, RoutedNets: 2, RouteScore: 0.5, PlacementScore: 0.5}
	candidate := placementRoutingRetryAttemptSummary{Attempt: 2, RoutingStatus: routing.StatusPartial, RoutedNets: 2, RouteScore: 0.7, PlacementScore: 0.5}
	if !placementRoutingAttemptBetter(candidate, current, RoutingRetryPolicySpec{}) {
		t.Fatalf("higher route score should rank better")
	}
	tiedLater := current
	tiedLater.Attempt = 3
	if placementRoutingAttemptBetter(tiedLater, current, RoutingRetryPolicySpec{}) {
		t.Fatalf("later tied attempt should not outrank earlier attempt")
	}
}

func TestPlacementRoutingAttemptSelectionReasons(t *testing.T) {
	previous := placementRoutingRetryAttemptSummary{Attempt: 1, RoutingStatus: routing.StatusPartial, RoutedNets: 1, DRCBlockingCount: 1}
	candidate := placementRoutingRetryAttemptSummary{Attempt: 2, RoutingStatus: routing.StatusPartial, RoutedNets: 1, DRCBlockingCount: 0}

	reason := placementRoutingAttemptSelectionReason(candidate, previous, RoutingRetryPolicySpec{DRCPolicy: RetryDRCPolicyRequired})
	if reason != "required_drc_cleaner" {
		t.Fatalf("selection reason = %q", reason)
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

func TestPlacementRoutingRetryAttemptSummaryDoesNotTreatRoutingIssuesAsBoardValidation(t *testing.T) {
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
	if summary.BoardValidationIssueCount != 0 || summary.BoardValidationBlocking != 0 {
		t.Fatalf("issue summary = %#v", summary)
	}
}

func TestBoardValidationCountsFromValidationStage(t *testing.T) {
	stage := StageResult{
		Name: StageValidation,
		Issues: []reports.Issue{
			{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning},
			{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked},
		},
	}

	total, blocking := boardValidationCountsFromRoutingStage(stage)
	if total != 2 || blocking != 1 {
		t.Fatalf("counts = %d/%d, want 2/1", total, blocking)
	}
}

func TestRetrySummaryAccessorsDecodeJSONMaps(t *testing.T) {
	stage := StageResult{Summary: map[string]any{
		"inter_block_routing": map[string]any{
			"complete_groups":  2,
			"partial_groups":   1,
			"proven_endpoints": 5,
		},
		"inter_block_route_trees": map[string]any{
			"groups_complete": 1,
			"groups_blocked":  2,
			"branches_routed": 3,
			"contact_misses":  4,
			"managed_nets":    []any{"SCL", "SDA"},
		},
		"inter_block_contacts": map[string]any{
			"contacts_proven": 10,
			"contact_misses":  2,
		},
		"route_tree_repair": map[string]any{
			"branch_failures":     4,
			"repairable_failures": 3,
			"hint_count":          3,
			"nets":                []any{"GND", "SDA"},
		},
	}}

	interBlock := retryInterBlockSummary(stage)
	if interBlock.CompleteGroups != 2 || interBlock.PartialGroups != 1 || interBlock.ProvenEndpoints != 5 {
		t.Fatalf("inter-block summary = %#v", interBlock)
	}
	routeTrees := retryRouteTreeSummary(stage)
	if routeTrees.GroupsComplete != 1 || routeTrees.GroupsBlocked != 2 || routeTrees.BranchesRouted != 3 || routeTrees.ContactMisses != 4 || len(routeTrees.ManagedNets) != 2 {
		t.Fatalf("route-tree summary = %#v", routeTrees)
	}
	contacts := retryInterBlockContactSummary(stage)
	if contacts.ContactsProven != 10 || contacts.ContactMisses != 2 {
		t.Fatalf("contact summary = %#v", contacts)
	}
	repair := retryRouteTreeRepairSummary(stage)
	if repair.BranchFailures != 4 || repair.RepairableFailures != 3 || repair.HintCount != 3 || len(repair.Nets) != 2 {
		t.Fatalf("repair summary = %#v", repair)
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
