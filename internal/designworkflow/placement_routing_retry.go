package designworkflow

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"math"
	"slices"
	"strconv"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
)

type placementRoutingRetrySummary struct {
	Enabled         bool                                  `json:"enabled"`
	Attempts        int                                   `json:"attempts"`
	Applied         int                                   `json:"applied"`
	StopReason      string                                `json:"stop_reason,omitempty"`
	SelectedAttempt int                                   `json:"selected_attempt,omitempty"`
	SelectedReason  string                                `json:"selected_reason,omitempty"`
	HintCategories  []string                              `json:"hint_categories,omitempty"`
	AttemptHistory  []placementRoutingRetryAttemptSummary `json:"attempt_history,omitempty"`
}

type placementRoutingRetryAttemptSummary struct {
	Attempt                   int                 `json:"attempt"`
	Placement                 map[string]any      `json:"placement,omitempty"`
	BaselineRoutingStatus     routing.Status      `json:"baseline_routing_status,omitempty"`
	BaselineRouteScore        float64             `json:"baseline_route_score,omitempty"`
	BaselineRoutedNets        int                 `json:"baseline_routed_nets"`
	BaselineFailedNets        int                 `json:"baseline_failed_nets"`
	RoutingStatus             routing.Status      `json:"routing_status,omitempty"`
	RouteScore                float64             `json:"route_score,omitempty"`
	RoutedNets                int                 `json:"routed_nets"`
	FailedNets                int                 `json:"failed_nets"`
	SkippedNets               int                 `json:"skipped_nets,omitempty"`
	PlacementScore            float64             `json:"placement_score,omitempty"`
	BoardValidationBlocking   int                 `json:"board_validation_blocking"`
	BoardValidationIssueCount int                 `json:"board_validation_issue_count"`
	DRCStatus                 retryEvidenceStatus `json:"drc_status,omitempty"`
	DRCIssueCount             int                 `json:"drc_issue_count,omitempty"`
	DRCBlockingCount          int                 `json:"drc_blocking_count,omitempty"`
	DRCSource                 string              `json:"drc_source,omitempty"`
	EligibleRefCount          int                 `json:"eligible_ref_count"`
	BlockedRefCount           int                 `json:"blocked_ref_count"`
	Selected                  bool                `json:"selected,omitempty"`
	SelectedReason            string              `json:"selected_reason,omitempty"`
	RegressionFlags           []string            `json:"regression_flags,omitempty"`
	RetryAdjustment           string              `json:"retry_adjustment,omitempty"`
	RouteTreeCompleteGroups   int                 `json:"route_tree_complete_groups,omitempty"`
	RouteTreePartialGroups    int                 `json:"route_tree_partial_groups,omitempty"`
	RouteTreeBlockedGroups    int                 `json:"route_tree_blocked_groups,omitempty"`
	RouteTreeProvenEndpoints  int                 `json:"route_tree_proven_endpoints,omitempty"`
	RouteTreeGraphComponents  int                 `json:"route_tree_graph_components,omitempty"`
	RouteTreeIncompleteNets   []string            `json:"route_tree_incomplete_nets,omitempty"`
	RouteTreeBranchesRouted   int                 `json:"route_tree_branches_routed,omitempty"`
	RouteTreeContactMisses    int                 `json:"route_tree_contact_misses,omitempty"`
	RouteTreeIssueCount       int                 `json:"route_tree_issue_count,omitempty"`
}

type retryEvidenceStatus string

const (
	retryEvidencePass    retryEvidenceStatus = "pass"
	retryEvidenceFail    retryEvidenceStatus = "fail"
	retryEvidenceMissing retryEvidenceStatus = "missing"
	retryEvidenceSkipped retryEvidenceStatus = "skipped"
	retryEvidenceWarning retryEvidenceStatus = "warning"
)

const retryScoreComparisonEpsilon = 1e-9
const retryScoreRelativeTolerance = 1e-6

func maybeRetryPlacementRouting(ctx context.Context, request Request, fragments PCBFragmentResult, placed PlacementStageResult, routed RoutingStageResult, routingOpts RoutingOptions, policy RoutingRetryPolicySpec) (PlacementStageResult, RoutingStageResult, placementRoutingRetrySummary) {
	return maybeRetryPlacementRoutingWithRouter(ctx, request, placed, routed, policy, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		return next, RoutePlacement(ctx, request, fragments, next, routingOpts)
	})
}

func maybeRetryExplicitPlacementRouting(ctx context.Context, request Request, placed PlacementStageResult, routed RoutingStageResult, routingOpts RoutingOptions, policy RoutingRetryPolicySpec) (PlacementStageResult, RoutingStageResult, placementRoutingRetrySummary) {
	return maybeRetryPlacementRoutingWithRouter(ctx, request, placed, routed, policy, func(next PlacementStageResult) (PlacementStageResult, RoutingStageResult) {
		return next, RouteExplicitCircuit(ctx, request, next, routingOpts)
	})
}

func maybeRetryPlacementRoutingWithRouter(ctx context.Context, request Request, placed PlacementStageResult, routed RoutingStageResult, policy RoutingRetryPolicySpec, routeNext func(PlacementStageResult) (PlacementStageResult, RoutingStageResult)) (PlacementStageResult, RoutingStageResult, placementRoutingRetrySummary) {
	summary := placementRoutingRetrySummary{Enabled: policy.Enabled, Attempts: 1}
	correctionReport := newAutonomousCorrectionReport(request, policy, placed, routed)
	if !policy.Enabled || policy.MaxAttempts <= 1 {
		summary.StopReason = "disabled"
		finalizeAutonomousCorrectionReport(correctionReport, request, summary)
		attachAutonomousCorrectionReport(&routed.Stage, correctionReport)
		return placed, routed, summary
	}
	bestPlaced := placed
	bestRouted := routed
	bestAttempt := placementRoutingAttemptSummaryForResult(1, nil, &placed, routed, "")
	bestAttempt.Placement = placed.Stage.Summary
	bestAttempt.Selected = true
	bestAttempt.SelectedReason = "initial_attempt"
	summary.AttemptHistory = append(summary.AttemptHistory, bestAttempt)
	currentPlaced := placed
	currentRouted := routed
	seenStates := map[string]struct{}{placementStateHash(currentPlaced.Result.Placements): {}}
	seenHintCategories := map[string]struct{}{}
	for attempt := 2; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			summary.StopReason = CorrectionStopContextCanceled
			break
		}
		if currentRouted.Result.Status == routing.StatusRouted {
			summary.StopReason = CorrectionStopRouted
			break
		}
		var hints []PlacementRetryHint
		var adjustedRequest placement.Request
		var adjustment PlacementRetryAdjustment
		var correctionPlan *AutonomousCorrectionPlan
		var correctionApplication *AutonomousCorrectionApplication
		if correctionReport != nil {
			diagnostics := BuildAutonomousCorrectionDiagnostics(currentPlaced.Stage.Issues, currentRouted.Stage.Issues)
			plan, err := PlanAutonomousCorrection(request, currentPlaced.Request, currentPlaced.Result.Placements, diagnostics, AutonomousCorrectionPlanOptions{
				Attempt: attempt, MaxAttempts: policy.MaxAttempts, AppliedRetryKeys: correctionReport.AppliedRetryKeys,
			})
			correctionPlan = &plan
			if err != nil {
				summary.StopReason = "correction_plan_error"
				correctionReport.AttemptHistory = append(correctionReport.AttemptHistory, autonomousCorrectionUnmaterializedAttempt(attempt, "planning_failed", correctionPlan, nil, currentPlaced))
				break
			}
			if !plan.Authorized {
				summary.StopReason = plan.StopReason
				correctionReport.AttemptHistory = append(correctionReport.AttemptHistory, autonomousCorrectionUnmaterializedAttempt(attempt, "plan_rejected", correctionPlan, nil, currentPlaced))
				break
			}
			adjusted, application, err := ApplyAutonomousCorrectionPlan(request, currentPlaced.Request, currentPlaced.Result.Placements, plan, correctionReport.AppliedRetryKeys)
			correctionApplication = &application
			if err != nil {
				summary.StopReason = "correction_apply_error"
				correctionApplication.StopReason = summary.StopReason
				correctionApplication.ProtectedInvariantsPreserved = false
				correctionReport.AttemptHistory = append(correctionReport.AttemptHistory, autonomousCorrectionUnmaterializedAttempt(attempt, "application_failed", correctionPlan, correctionApplication, currentPlaced))
				break
			}
			if !application.Applied {
				summary.StopReason = application.StopReason
				correctionReport.AttemptHistory = append(correctionReport.AttemptHistory, autonomousCorrectionUnmaterializedAttempt(attempt, "application_rejected", correctionPlan, correctionApplication, currentPlaced))
				break
			}
			adjustedRequest = adjusted
			adjustment = application.Adjustment
			correctionReport.AppliedRetryKeys = append(correctionReport.AppliedRetryKeys, plan.RetryKey)
			hints = autonomousCorrectionPlacementHints(plan.Actions)
		} else {
			diagnostics := routing.DiagnosticsForResult(currentRouted.Result)
			hints = filterPlacementRetryHints(BuildPlacementRetryHints(diagnostics, currentPlaced.Result.Quality), policy)
			routeTreeHints := filterPlacementRetryHints(BuildRouteTreePlacementRetryHints(currentRouted.Stage.Issues), policy)
			hints = mergePlacementRetryHints(hints, routeTreeHints...)
			adjustedRequest, adjustment = BuildPlacementRetryAdjustment(currentPlaced.Request, hints, attempt-1)
		}
		for _, category := range placementRetryHintCategoryStrings(hints) {
			if _, ok := seenHintCategories[category]; ok {
				continue
			}
			seenHintCategories[category] = struct{}{}
			summary.HintCategories = append(summary.HintCategories, category)
		}
		if correctionReport == nil && len(hints) == 0 {
			summary.StopReason = "no_eligible_hints"
			break
		}
		if correctionReport == nil && !adjustment.Applied {
			summary.StopReason = "no_safe_adjustment"
			break
		}
		nextPlaced := placeAdjustedRequest(ctx, adjustedRequest)
		preserveRetryPlacementEvidence(&nextPlaced.Stage, currentPlaced.Stage)
		if workflowStageBlocked(nextPlaced.Stage) {
			summary.StopReason = "placement_blocked"
			if correctionReport != nil {
				correctionReport.AttemptHistory = append(correctionReport.AttemptHistory, autonomousCorrectionAttemptForResult(attempt, correctionPlan, correctionApplication, nextPlaced, RoutingStageResult{}))
			}
			break
		}
		stateHash := placementStateHash(nextPlaced.Result.Placements)
		if _, ok := seenStates[stateHash]; ok {
			summary.StopReason = CorrectionStopRepeatedPlacementState
			if correctionReport != nil {
				correctionReport.AttemptHistory = append(correctionReport.AttemptHistory, autonomousCorrectionAttemptForResult(attempt, correctionPlan, correctionApplication, nextPlaced, RoutingStageResult{}))
			}
			break
		}
		seenStates[stateHash] = struct{}{}
		nextPlaced, nextRouted := routeNext(nextPlaced)
		if correctionReport != nil {
			correctionReport.AttemptHistory = append(correctionReport.AttemptHistory, autonomousCorrectionAttemptForResult(attempt, correctionPlan, correctionApplication, nextPlaced, nextRouted))
		}
		summary.Attempts = attempt
		summary.Applied++
		ensureStageSummary(&nextRouted.Stage)
		adjustmentSummary := PlacementRetryAdjustmentSummary(adjustment)
		nextRouted.Stage.Summary["retry_adjustment"] = adjustmentSummary
		attemptSummary := placementRoutingAttemptSummaryForResult(attempt, &currentRouted, &nextPlaced, nextRouted, adjustmentSummary)
		attemptSummary.Placement = nextPlaced.Stage.Summary
		attemptSummary.EligibleRefCount = adjustment.EligibleRefs
		attemptSummary.BlockedRefCount = adjustment.BlockedRefs
		attemptSummary.RegressionFlags = placementRoutingRegressionFlags(attemptSummary, bestAttempt)
		improved := placementRoutingAttemptBetter(attemptSummary, bestAttempt, policy)
		if improved {
			bestPlaced = nextPlaced
			bestRouted = nextRouted
			attemptSummary.SelectedReason = placementRoutingAttemptSelectionReason(attemptSummary, bestAttempt, policy)
			bestAttempt = attemptSummary
		}
		summary.AttemptHistory = append(summary.AttemptHistory, attemptSummary)
		if !improved && policy.StopOnNewBlockers && slices.Contains(attemptSummary.RegressionFlags, CorrectionStopDRCRegression) {
			summary.StopReason = CorrectionStopDRCRegression
			break
		} else if !improved && policy.StopOnNewBlockers && slices.Contains(attemptSummary.RegressionFlags, CorrectionStopBoardValidationRegression) {
			summary.StopReason = CorrectionStopBoardValidationRegression
			break
		} else if !improved && policy.StopOnNonImprovement {
			summary.StopReason = CorrectionStopNonImprovingRetry
			break
		}
		currentPlaced = nextPlaced
		currentRouted = nextRouted
	}
	if summary.StopReason == "" {
		summary.StopReason = CorrectionStopMaxAttempts
	}
	summary.SelectedAttempt = bestAttempt.Attempt
	summary.SelectedReason = bestAttempt.SelectedReason
	markSelectedRetryAttempt(summary.AttemptHistory, bestAttempt)
	ensureStageSummary(&bestRouted.Stage)
	bestRouted.Stage.Summary["routing_retry"] = summary
	finalizeAutonomousCorrectionReport(correctionReport, request, summary)
	attachAutonomousCorrectionReport(&bestRouted.Stage, correctionReport)
	return bestPlaced, bestRouted, summary
}

func attachAutonomousCorrectionReport(stage *StageResult, report *AutonomousCorrectionReport) {
	if stage == nil || report == nil {
		return
	}
	ensureStageSummary(stage)
	stage.Summary["autonomous_correction"] = report
}

func ensureStageSummary(stage *StageResult) {
	if stage.Summary == nil {
		stage.Summary = map[string]any{}
	}
}

func placeAdjustedRequest(ctx context.Context, request placement.Request) PlacementStageResult {
	result := placement.PlaceContext(ctx, request)
	stage := NewStageResult(StagePlacement, result.Issues)
	mobilitySummary := placement.MobilitySummaryForComponents(request.Components)
	stage.Summary = map[string]any{
		"component_count": result.Metrics.ComponentCount,
		"placed_count":    result.Metrics.PlacedCount,
		"unplaced_count":  result.Metrics.UnplacedCount,
		"fixed_count":     result.Metrics.FixedCount,
		"retry":           true,
		"mobility":        mobilitySummary,
	}
	if scoring := placementCandidateScoringSummary(result.CandidateScoring); scoring != nil {
		stage.Summary["candidate_scoring"] = scoring
	}
	if result.Status != placement.StatusPlaced && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return PlacementStageResult{Request: request, Result: result, Stage: stage}
}

func preserveRetryPlacementEvidence(next *StageResult, current StageResult) {
	if next == nil || current.Summary == nil {
		return
	}
	ensureStageSummary(next)
	for _, key := range []string{"pad_hydration", "candidate_scoring"} {
		if _, exists := next.Summary[key]; exists {
			continue
		}
		if value, ok := current.Summary[key]; ok {
			next.Summary[key] = value
		}
	}
}

func filterPlacementRetryHints(hints []PlacementRetryHint, policy RoutingRetryPolicySpec) []PlacementRetryHint {
	allowed := map[PlacementRetryHintCategory]struct{}{}
	for _, category := range policy.AllowedHintCategories {
		allowed[category] = struct{}{}
	}
	var filtered []PlacementRetryHint
	for _, hint := range hints {
		if !hint.RetryEligible {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[hint.Category]; !ok {
				continue
			}
		}
		filtered = append(filtered, hint)
	}
	return filtered
}

func mergePlacementRetryHints(base []PlacementRetryHint, extra ...PlacementRetryHint) []PlacementRetryHint {
	seen := map[string]struct{}{}
	out := make([]PlacementRetryHint, 0, len(base)+len(extra))
	for _, hint := range base {
		key := placementRetryHintKey(hint)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, hint)
	}
	for _, hint := range extra {
		key := placementRetryHintKey(hint)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, hint)
	}
	return out
}

func placementRetryHintCategoryStrings(hints []PlacementRetryHint) []string {
	seen := map[string]struct{}{}
	var categories []string
	for _, hint := range hints {
		category := string(hint.Category)
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		categories = append(categories, category)
	}
	return categories
}

func routingAttemptBetter(candidate RoutingStageResult, current RoutingStageResult) bool {
	candidateSummary := placementRoutingAttemptSummaryForResult(0, nil, nil, candidate, "")
	currentSummary := placementRoutingAttemptSummaryForResult(0, nil, nil, current, "")
	return placementRoutingAttemptBetter(candidateSummary, currentSummary, RoutingRetryPolicySpec{})
}

func placementRoutingAttemptSummaryForResult(attempt int, baseline *RoutingStageResult, placed *PlacementStageResult, routed RoutingStageResult, adjustment string) placementRoutingRetryAttemptSummary {
	summary := placementRoutingRetryAttemptSummary{
		Attempt:                 attempt,
		RoutingStatus:           routed.Result.Status,
		RouteScore:              routeQualityScore(routed),
		RoutedNets:              routed.Result.Metrics.RoutedNetCount,
		FailedNets:              routed.Result.Metrics.FailedNetCount,
		SkippedNets:             skippedNetCount(routed),
		PlacementScore:          placementQualityScore(placed),
		DRCStatus:               retryEvidenceSkipped,
		DRCSource:               "skipped",
		RetryAdjustment:         adjustment,
		RegressionFlags:         nil,
		BoardValidationBlocking: 0,
	}
	summary.BoardValidationIssueCount, summary.BoardValidationBlocking = boardValidationCountsFromRoutingStage(routed.Stage)
	routeSummary := retryInterBlockSummary(routed.Stage)
	treeSummary := retryRouteTreeSummary(routed.Stage)
	contactSummary := retryInterBlockContactSummary(routed.Stage)
	repairSummary := retryRouteTreeRepairSummary(routed.Stage)
	contactGraph := retryRouteTreeContactGraphSummary(routed.Stage)
	summary.RouteTreeCompleteGroups = routeSummary.CompleteGroups
	summary.RouteTreePartialGroups = routeSummary.PartialGroups
	summary.RouteTreeBlockedGroups = routeSummary.BlockedGroups
	summary.RouteTreeProvenEndpoints = routeSummary.ProvenEndpoints
	summary.RouteTreeGraphComponents = contactGraph.Components
	summary.RouteTreeIncompleteNets = routeTreeIncompleteNets(contactGraph)
	summary.RouteTreeBranchesRouted = treeSummary.BranchesRouted
	summary.RouteTreeContactMisses = contactSummary.ContactMisses
	summary.RouteTreeIssueCount = repairSummary.BranchFailures
	if baseline != nil {
		summary.BaselineRoutingStatus = baseline.Result.Status
		summary.BaselineRouteScore = routeQualityScore(*baseline)
		summary.BaselineRoutedNets = baseline.Result.Metrics.RoutedNetCount
		summary.BaselineFailedNets = baseline.Result.Metrics.FailedNetCount
	} else {
		summary.BaselineRoutingStatus = summary.RoutingStatus
		summary.BaselineRouteScore = summary.RouteScore
		summary.BaselineRoutedNets = summary.RoutedNets
		summary.BaselineFailedNets = summary.FailedNets
	}
	return normalizePlacementRoutingRetryAttempt(summary)
}

func normalizePlacementRoutingRetryAttempt(summary placementRoutingRetryAttemptSummary) placementRoutingRetryAttemptSummary {
	if summary.Attempt < 0 {
		summary.Attempt = 0
	}
	if math.IsNaN(summary.RouteScore) || math.IsInf(summary.RouteScore, 0) {
		summary.RouteScore = 0
	}
	if math.IsNaN(summary.BaselineRouteScore) || math.IsInf(summary.BaselineRouteScore, 0) {
		summary.BaselineRouteScore = 0
	}
	if math.IsNaN(summary.PlacementScore) || math.IsInf(summary.PlacementScore, 0) {
		summary.PlacementScore = 0
	}
	if summary.DRCStatus == "" {
		summary.DRCStatus = retryEvidenceSkipped
	}
	if summary.DRCSource == "" {
		summary.DRCSource = string(summary.DRCStatus)
	}
	if summary.SkippedNets < 0 {
		summary.SkippedNets = 0
	}
	return summary
}

func routeQualityScore(routed RoutingStageResult) float64 {
	if routed.Result.Quality == nil {
		return 0
	}
	return routed.Result.Quality.Score.Overall
}

func placementQualityScore(placed *PlacementStageResult) float64 {
	if placed == nil || placed.Result.Quality == nil {
		return 0
	}
	return placed.Result.Quality.Score.Total
}

func skippedNetCount(routed RoutingStageResult) int {
	total := len(routed.Request.Nets)
	if total == 0 {
		return 0
	}
	skipped := total - routed.Result.Metrics.RoutedNetCount - routed.Result.Metrics.FailedNetCount
	if skipped < 0 {
		return 0
	}
	return skipped
}

func summarizeRetryIssues(issues []reports.Issue) (total int, blocking int) {
	for _, issue := range issues {
		total++
		if issue.Blocking() {
			blocking++
		}
	}
	return total, blocking
}

func boardValidationCountsFromRoutingStage(stage StageResult) (total int, blocking int) {
	if stage.Summary != nil {
		total = intFromRetrySummary(stage.Summary, "board_validation_issue_count")
		blocking = intFromRetrySummary(stage.Summary, "board_validation_blocking")
		if total > 0 || blocking > 0 {
			return total, blocking
		}
	}
	if stage.Name == StageValidation {
		return summarizeRetryIssues(stage.Issues)
	}
	return 0, 0
}

func retryInterBlockSummary(stage StageResult) InterBlockRouteCompletionSummary {
	value, ok := retrySummaryMapValue(stage, "inter_block_routing")
	if !ok {
		return InterBlockRouteCompletionSummary{}
	}
	if summary, ok := value.(InterBlockRouteCompletionSummary); ok {
		return summary
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return InterBlockRouteCompletionSummary{}
	}
	return InterBlockRouteCompletionSummary{
		CompleteGroups:    intFromSummaryValue(fields["complete_groups"]),
		PartialGroups:     intFromSummaryValue(fields["partial_groups"]),
		BlockedGroups:     intFromSummaryValue(fields["blocked_groups"]),
		ProvenEndpoints:   intFromSummaryValue(fields["proven_endpoints"]),
		BranchesCompleted: intFromSummaryValue(fields["branches_completed"]),
	}
}

func retryRouteTreeSummary(stage StageResult) InterBlockRouteTreeExecutionSummary {
	value, ok := retrySummaryMapValue(stage, "inter_block_route_trees")
	if !ok {
		return InterBlockRouteTreeExecutionSummary{}
	}
	if summary, ok := value.(InterBlockRouteTreeExecutionSummary); ok {
		return summary
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return InterBlockRouteTreeExecutionSummary{}
	}
	return InterBlockRouteTreeExecutionSummary{
		GroupsComplete: intFromSummaryValue(fields["groups_complete"]),
		GroupsPartial:  intFromSummaryValue(fields["groups_partial"]),
		GroupsBlocked:  intFromSummaryValue(fields["groups_blocked"]),
		BranchesRouted: intFromSummaryValue(fields["branches_routed"]),
		ContactMisses:  intFromSummaryValue(fields["contact_misses"]),
		IssueCount:     intFromSummaryValue(fields["issue_count"]),
		ManagedNets:    stringsFromSummaryValue(fields["managed_nets"]),
	}
}

func retryRouteTreeContactGraphSummary(stage StageResult) RouteTreeContactGraphSummary {
	value, ok := retrySummaryMapValue(stage, "route_tree_contact_graph")
	if !ok {
		return RouteTreeContactGraphSummary{}
	}
	if summary, ok := value.(RouteTreeContactGraphSummary); ok {
		return summary
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return RouteTreeContactGraphSummary{}
	}
	summary := RouteTreeContactGraphSummary{Components: intFromSummaryValue(fields["components"])}
	for _, raw := range summarySliceValue(fields["groups"]) {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := group["net_name"].(string)
		status, _ := group["status"].(string)
		if name == "" || status == "" {
			continue
		}
		summary.Groups = append(summary.Groups, RouteTreeContactGraphGroupSummary{
			NetName: name,
			Status:  RouteTreeContactGraphGroupStatus(status),
		})
	}
	return summary
}

func summarySliceValue(value any) []any {
	switch values := value.(type) {
	case []any:
		return values
	case []RouteTreeContactGraphGroupSummary:
		out := make([]any, len(values))
		for index := range values {
			out[index] = values[index]
		}
		return out
	default:
		return nil
	}
}

func routeTreeIncompleteNets(summary RouteTreeContactGraphSummary) []string {
	var nets []string
	for _, group := range summary.Groups {
		if group.NetName == "" || group.Status == RouteTreeContactGraphGroupComplete {
			continue
		}
		nets = append(nets, group.NetName)
	}
	slices.Sort(nets)
	return slices.Compact(nets)
}

func retryInterBlockContactSummary(stage StageResult) InterBlockContactSummary {
	value, ok := retrySummaryMapValue(stage, "inter_block_contacts")
	if !ok {
		return InterBlockContactSummary{}
	}
	if summary, ok := value.(InterBlockContactSummary); ok {
		return summary
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return InterBlockContactSummary{}
	}
	return InterBlockContactSummary{
		ContactsProven: intFromSummaryValue(fields["contacts_proven"]),
		ContactMisses:  intFromSummaryValue(fields["contact_misses"]),
	}
}

func retryRouteTreeRepairSummary(stage StageResult) InterBlockRouteTreeRepairSummary {
	value, ok := retrySummaryMapValue(stage, "route_tree_repair")
	if !ok {
		return InterBlockRouteTreeRepairSummary{}
	}
	if summary, ok := value.(InterBlockRouteTreeRepairSummary); ok {
		return summary
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return InterBlockRouteTreeRepairSummary{}
	}
	return InterBlockRouteTreeRepairSummary{
		BranchFailures:     intFromSummaryValue(fields["branch_failures"]),
		RepairableFailures: intFromSummaryValue(fields["repairable_failures"]),
		HintCount:          intFromSummaryValue(fields["hint_count"]),
		Nets:               stringsFromSummaryValue(fields["nets"]),
	}
}

func retrySummaryMapValue(stage StageResult, key string) (any, bool) {
	if stage.Summary == nil {
		return nil, false
	}
	value, ok := stage.Summary[key]
	if !ok || value == nil {
		return nil, false
	}
	return value, true
}

func stringsFromSummaryValue(value any) []string {
	switch values := value.(type) {
	case []string:
		return slices.Clone(values)
	case []any:
		out := make([]string, 0, len(values))
		for _, raw := range values {
			text, ok := raw.(string)
			if ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func placementRoutingAttemptBetter(candidate, current placementRoutingRetryAttemptSummary, policy RoutingRetryPolicySpec) bool {
	return comparePlacementRoutingAttempts(candidate, current, policy) > 0
}

func comparePlacementRoutingAttempts(candidate, current placementRoutingRetryAttemptSummary, policy RoutingRetryPolicySpec) int {
	compare, _ := placementRoutingAttemptComparison(candidate, current, policy)
	return compare
}

func placementRoutingAttemptComparison(candidate, current placementRoutingRetryAttemptSummary, policy RoutingRetryPolicySpec) (int, string) {
	if routeTreeRegressesCompleteNet(candidate.RouteTreeIncompleteNets, current.RouteTreeIncompleteNets) {
		return -1, "regresses_route_tree_complete_net"
	}
	if policy.DRCPolicy == RetryDRCPolicyRequired {
		candidateDRCOK := candidate.DRCBlockingCount == 0 && candidate.DRCStatus != retryEvidenceFail
		currentDRCOK := current.DRCBlockingCount == 0 && current.DRCStatus != retryEvidenceFail
		if candidateDRCOK != currentDRCOK {
			if candidateDRCOK {
				return 1, "required_drc_cleaner"
			}
			return -1, "required_drc_cleaner"
		}
	}
	if candidate.BoardValidationBlocking != current.BoardValidationBlocking {
		return lowerIsBetter(candidate.BoardValidationBlocking, current.BoardValidationBlocking), "fewer_board_validation_blockers"
	}
	if candidate.DRCBlockingCount != current.DRCBlockingCount {
		return lowerIsBetter(candidate.DRCBlockingCount, current.DRCBlockingCount), "fewer_drc_blockers"
	}
	if candidate.RouteTreeBlockedGroups != current.RouteTreeBlockedGroups {
		return lowerIsBetter(candidate.RouteTreeBlockedGroups, current.RouteTreeBlockedGroups), "fewer_route_tree_blocked_groups"
	}
	if candidate.RouteTreeCompleteGroups != current.RouteTreeCompleteGroups {
		return higherIsBetter(candidate.RouteTreeCompleteGroups, current.RouteTreeCompleteGroups), "more_route_tree_complete_groups"
	}
	if candidate.RouteTreeProvenEndpoints != current.RouteTreeProvenEndpoints {
		return higherIsBetter(candidate.RouteTreeProvenEndpoints, current.RouteTreeProvenEndpoints), "more_route_tree_proven_endpoints"
	}
	if candidate.RouteTreeGraphComponents != current.RouteTreeGraphComponents {
		return lowerIsBetter(candidate.RouteTreeGraphComponents, current.RouteTreeGraphComponents), "fewer_route_tree_graph_components"
	}
	if len(candidate.RouteTreeIncompleteNets) != len(current.RouteTreeIncompleteNets) {
		return lowerIsBetter(len(candidate.RouteTreeIncompleteNets), len(current.RouteTreeIncompleteNets)), "fewer_route_tree_incomplete_nets"
	}
	if candidate.RouteTreeBranchesRouted != current.RouteTreeBranchesRouted {
		return higherIsBetter(candidate.RouteTreeBranchesRouted, current.RouteTreeBranchesRouted), "more_route_tree_routed_branches"
	}
	if candidate.RouteTreeContactMisses != current.RouteTreeContactMisses {
		return lowerIsBetter(candidate.RouteTreeContactMisses, current.RouteTreeContactMisses), "fewer_route_tree_contact_misses"
	}
	if candidate.RouteTreeIssueCount != current.RouteTreeIssueCount {
		return lowerIsBetter(candidate.RouteTreeIssueCount, current.RouteTreeIssueCount), "fewer_route_tree_issues"
	}
	if routingStatusRank(candidate.RoutingStatus) != routingStatusRank(current.RoutingStatus) {
		return higherIsBetter(routingStatusRank(candidate.RoutingStatus), routingStatusRank(current.RoutingStatus)), "routing_status_improved"
	}
	if candidate.FailedNets != current.FailedNets {
		return lowerIsBetter(candidate.FailedNets, current.FailedNets), "fewer_failed_nets"
	}
	if candidate.RoutedNets != current.RoutedNets {
		return higherIsBetter(candidate.RoutedNets, current.RoutedNets), "more_routed_nets"
	}
	if compare := compareRouteScoreWithPolicy(candidate.RouteScore, current.RouteScore, policy); compare != 0 {
		return compare, "higher_route_quality"
	}
	return lowerIsBetter(candidate.Attempt, current.Attempt), "best_ranked_attempt"
}

func routeTreeRegressesCompleteNet(candidateIncomplete []string, currentIncomplete []string) bool {
	candidateSet := make(map[string]struct{}, len(candidateIncomplete))
	for _, netName := range candidateIncomplete {
		candidateSet[netName] = struct{}{}
	}
	currentSet := make(map[string]struct{}, len(currentIncomplete))
	for _, netName := range currentIncomplete {
		currentSet[netName] = struct{}{}
	}
	for netName := range candidateSet {
		if _, alreadyIncomplete := currentSet[netName]; !alreadyIncomplete {
			return true
		}
	}
	return false
}

func compareRouteScoreWithPolicy(candidate float64, current float64, policy RoutingRetryPolicySpec) int {
	delta := candidate - current
	epsilon := routeScoreComparisonEpsilon(candidate, current)
	if delta < -epsilon {
		return -1
	}
	threshold := policy.MinRoutingScoreDelta
	if threshold <= 0 {
		if delta > epsilon {
			return 1
		}
		return 0
	}
	tolerance := threshold * retryScoreRelativeTolerance
	if delta >= threshold-tolerance {
		return 1
	}
	return 0
}

func routeScoreComparisonEpsilon(candidate float64, current float64) float64 {
	scale := math.Max(math.Abs(candidate), math.Abs(current))
	if scale < 1 {
		scale = 1
	}
	return retryScoreComparisonEpsilon * scale
}

func placementRoutingAttemptSelectionReason(candidate, previousBest placementRoutingRetryAttemptSummary, policy RoutingRetryPolicySpec) string {
	compare, reason := placementRoutingAttemptComparison(candidate, previousBest, policy)
	if compare <= 0 {
		return "best_ranked_attempt"
	}
	return reason
}

func lowerIsBetter(candidate, current int) int {
	switch {
	case candidate < current:
		return 1
	case candidate > current:
		return -1
	default:
		return 0
	}
}

func higherIsBetter(candidate, current int) int {
	switch {
	case candidate > current:
		return 1
	case candidate < current:
		return -1
	default:
		return 0
	}
}

func higherFloatIsBetter(candidate, current float64) int {
	switch {
	case candidate > current:
		return 1
	case candidate < current:
		return -1
	default:
		return 0
	}
}

func intFromRetrySummary(summary map[string]any, key string) int {
	if summary == nil {
		return 0
	}
	return intFromSummaryValue(summary[key])
}

func intFromSummaryValue(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float32:
		return int(math.Round(float64(value)))
	case float64:
		return int(math.Round(value))
	default:
		return 0
	}
}

func placementRoutingRegressionFlags(candidate, currentBest placementRoutingRetryAttemptSummary) []string {
	var flags []string
	if candidate.BoardValidationBlocking > currentBest.BoardValidationBlocking {
		flags = append(flags, "board_validation_regression")
	}
	if candidate.DRCBlockingCount > currentBest.DRCBlockingCount {
		flags = append(flags, "drc_regression")
	}
	if candidate.RouteScore < currentBest.RouteScore {
		flags = append(flags, "route_quality_regression")
	}
	return flags
}

func markSelectedRetryAttempt(history []placementRoutingRetryAttemptSummary, selected placementRoutingRetryAttemptSummary) {
	for index := range history {
		history[index].Selected = false
		history[index].SelectedReason = ""
		if history[index].Attempt != selected.Attempt {
			continue
		}
		history[index].Selected = true
		history[index].SelectedReason = selected.SelectedReason
	}
}

func routingStatusRank(status routing.Status) int {
	switch status {
	case routing.StatusRouted:
		return 3
	case routing.StatusPartial:
		return 2
	case routing.StatusBlocked:
		return 1
	default:
		return 0
	}
}

func placementStateHash(placements []placement.PlacementResult) string {
	refs := make([]string, 0, len(placements))
	byRef := map[string]placement.PlacementResult{}
	seen := map[string]struct{}{}
	for _, result := range placements {
		if result.Fixed {
			continue
		}
		if _, ok := seen[result.Ref]; !ok {
			refs = append(refs, result.Ref)
			seen[result.Ref] = struct{}{}
			byRef[result.Ref] = result
		}
	}
	slices.Sort(refs)
	hash := fnv.New64a()
	for _, ref := range refs {
		pos := byRef[ref].Position
		var buf [64]byte
		writeHashBytes(hash, []byte(ref))
		writeHashBytes(hash, strconv.AppendFloat(buf[:0], pos.XMM, 'f', 4, 64))
		writeHashBytes(hash, strconv.AppendFloat(buf[:0], pos.YMM, 'f', 4, 64))
		writeHashBytes(hash, strconv.AppendFloat(buf[:0], pos.RotationDeg, 'f', 2, 64))
		writeHashBytes(hash, []byte(pos.Layer))
	}
	return strconv.FormatUint(hash.Sum64(), 16)
}

func writeHashBytes(hash interface{ Write([]byte) (int, error) }, value []byte) {
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write(value)
}
