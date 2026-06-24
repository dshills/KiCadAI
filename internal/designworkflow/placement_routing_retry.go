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
}

type retryEvidenceStatus string

const (
	retryEvidencePass    retryEvidenceStatus = "pass"
	retryEvidenceFail    retryEvidenceStatus = "fail"
	retryEvidenceMissing retryEvidenceStatus = "missing"
	retryEvidenceSkipped retryEvidenceStatus = "skipped"
	retryEvidenceWarning retryEvidenceStatus = "warning"
)

func maybeRetryPlacementRouting(ctx context.Context, request Request, fragments PCBFragmentResult, placed PlacementStageResult, routed RoutingStageResult, routingOpts RoutingOptions, policy RoutingRetryPolicySpec) (PlacementStageResult, RoutingStageResult, placementRoutingRetrySummary) {
	summary := placementRoutingRetrySummary{Enabled: policy.Enabled, Attempts: 1}
	if !policy.Enabled || policy.MaxAttempts <= 1 {
		summary.StopReason = "disabled"
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
			summary.StopReason = "context_canceled"
			break
		}
		if currentRouted.Result.Status == routing.StatusRouted {
			summary.StopReason = "routed"
			break
		}
		diagnostics := routing.DiagnosticsForResult(currentRouted.Result)
		hints := filterPlacementRetryHints(BuildPlacementRetryHints(diagnostics, currentPlaced.Result.Quality), policy)
		for _, category := range placementRetryHintCategoryStrings(hints) {
			if _, ok := seenHintCategories[category]; ok {
				continue
			}
			seenHintCategories[category] = struct{}{}
			summary.HintCategories = append(summary.HintCategories, category)
		}
		if len(hints) == 0 {
			summary.StopReason = "no_eligible_hints"
			break
		}
		adjustedRequest, adjustment := BuildPlacementRetryAdjustment(currentPlaced.Request, hints, attempt-1)
		if !adjustment.Applied {
			summary.StopReason = "no_safe_adjustment"
			break
		}
		nextPlaced := placeAdjustedRequest(ctx, adjustedRequest)
		preserveRetryPlacementEvidence(&nextPlaced.Stage, currentPlaced.Stage)
		if workflowStageBlocked(nextPlaced.Stage) {
			summary.StopReason = "placement_blocked"
			break
		}
		stateHash := placementStateHash(nextPlaced.Result.Placements)
		if _, ok := seenStates[stateHash]; ok {
			summary.StopReason = "repeated_placement_state"
			break
		}
		seenStates[stateHash] = struct{}{}
		nextRouted := RoutePlacement(ctx, request, fragments, nextPlaced, routingOpts)
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
		if !improved && policy.StopOnNewBlockers && slices.Contains(attemptSummary.RegressionFlags, "drc_regression") {
			summary.StopReason = "drc_regression"
			break
		} else if !improved && policy.StopOnNewBlockers && slices.Contains(attemptSummary.RegressionFlags, "board_validation_regression") {
			summary.StopReason = "board_validation_regression"
			break
		} else if !improved && policy.StopOnNonImprovement {
			summary.StopReason = "non_improving_retry"
			break
		}
		currentPlaced = nextPlaced
		currentRouted = nextRouted
	}
	if summary.StopReason == "" {
		summary.StopReason = "max_attempts"
	}
	summary.SelectedAttempt = bestAttempt.Attempt
	summary.SelectedReason = bestAttempt.SelectedReason
	markSelectedRetryAttempt(summary.AttemptHistory, bestAttempt)
	ensureStageSummary(&bestRouted.Stage)
	bestRouted.Stage.Summary["routing_retry"] = summary
	return bestPlaced, bestRouted, summary
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

func placementRoutingAttemptBetter(candidate, current placementRoutingRetryAttemptSummary, policy RoutingRetryPolicySpec) bool {
	return comparePlacementRoutingAttempts(candidate, current, policy) > 0
}

func comparePlacementRoutingAttempts(candidate, current placementRoutingRetryAttemptSummary, policy RoutingRetryPolicySpec) int {
	if policy.DRCPolicy == RetryDRCPolicyRequired {
		candidateDRCOK := candidate.DRCBlockingCount == 0 && candidate.DRCStatus != retryEvidenceFail
		currentDRCOK := current.DRCBlockingCount == 0 && current.DRCStatus != retryEvidenceFail
		if candidateDRCOK != currentDRCOK {
			if candidateDRCOK {
				return 1
			}
			return -1
		}
	}
	if candidate.BoardValidationBlocking != current.BoardValidationBlocking {
		return lowerIsBetter(candidate.BoardValidationBlocking, current.BoardValidationBlocking)
	}
	if candidate.DRCBlockingCount != current.DRCBlockingCount {
		return lowerIsBetter(candidate.DRCBlockingCount, current.DRCBlockingCount)
	}
	if routingStatusRank(candidate.RoutingStatus) != routingStatusRank(current.RoutingStatus) {
		return higherIsBetter(routingStatusRank(candidate.RoutingStatus), routingStatusRank(current.RoutingStatus))
	}
	if candidate.FailedNets != current.FailedNets {
		return lowerIsBetter(candidate.FailedNets, current.FailedNets)
	}
	if candidate.RoutedNets != current.RoutedNets {
		return higherIsBetter(candidate.RoutedNets, current.RoutedNets)
	}
	if candidate.RouteScore != current.RouteScore {
		return higherFloatIsBetter(candidate.RouteScore, current.RouteScore)
	}
	return lowerIsBetter(candidate.Attempt, current.Attempt)
}

func placementRoutingAttemptSelectionReason(candidate, previousBest placementRoutingRetryAttemptSummary, policy RoutingRetryPolicySpec) string {
	switch {
	case policy.DRCPolicy == RetryDRCPolicyRequired && candidate.DRCBlockingCount == 0 && previousBest.DRCBlockingCount > 0:
		return "required_drc_cleaner"
	case candidate.BoardValidationBlocking < previousBest.BoardValidationBlocking:
		return "fewer_board_validation_blockers"
	case candidate.DRCBlockingCount < previousBest.DRCBlockingCount:
		return "fewer_drc_blockers"
	case routingStatusRank(candidate.RoutingStatus) > routingStatusRank(previousBest.RoutingStatus):
		return "routing_status_improved"
	case candidate.FailedNets < previousBest.FailedNets:
		return "fewer_failed_nets"
	case candidate.RoutedNets > previousBest.RoutedNets:
		return "more_routed_nets"
	case candidate.RouteScore > previousBest.RouteScore:
		return "higher_route_quality"
	default:
		return "best_ranked_attempt"
	}
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
	switch value := summary[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
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
