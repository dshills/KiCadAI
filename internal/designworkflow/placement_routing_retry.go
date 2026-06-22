package designworkflow

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"slices"
	"strconv"

	"kicadai/internal/placement"
	"kicadai/internal/routing"
)

type placementRoutingRetrySummary struct {
	Enabled        bool             `json:"enabled"`
	Attempts       int              `json:"attempts"`
	Applied        int              `json:"applied"`
	StopReason     string           `json:"stop_reason,omitempty"`
	HintCategories []string         `json:"hint_categories,omitempty"`
	AttemptHistory []map[string]any `json:"attempt_history,omitempty"`
}

func maybeRetryPlacementRouting(ctx context.Context, request Request, fragments PCBFragmentResult, placed PlacementStageResult, routed RoutingStageResult, routingOpts RoutingOptions, policy RoutingRetryPolicySpec) (PlacementStageResult, RoutingStageResult, placementRoutingRetrySummary) {
	summary := placementRoutingRetrySummary{Enabled: policy.Enabled, Attempts: 1}
	if !policy.Enabled || policy.MaxAttempts <= 1 {
		summary.StopReason = "disabled"
		return placed, routed, summary
	}
	bestPlaced := placed
	bestRouted := routed
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
		nextRouted.Stage.Summary["retry_adjustment"] = PlacementRetryAdjustmentSummary(adjustment)
		summary.AttemptHistory = append(summary.AttemptHistory, map[string]any{
			"attempt":                 attempt,
			"placement":               nextPlaced.Stage.Summary,
			"baseline_routing_status": currentRouted.Result.Status,
			"baseline_failed_nets":    currentRouted.Result.Metrics.FailedNetCount,
			"baseline_routed_nets":    currentRouted.Result.Metrics.RoutedNetCount,
			"routing_status":          nextRouted.Result.Status,
			"failed_nets":             nextRouted.Result.Metrics.FailedNetCount,
			"routed_nets":             nextRouted.Result.Metrics.RoutedNetCount,
		})
		if routingAttemptBetter(nextRouted, bestRouted) {
			bestPlaced = nextPlaced
			bestRouted = nextRouted
		} else if policy.StopOnNonImprovement {
			summary.StopReason = "non_improving_retry"
			break
		}
		currentPlaced = nextPlaced
		currentRouted = nextRouted
	}
	if summary.StopReason == "" {
		summary.StopReason = "max_attempts"
	}
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
	stage.Summary = map[string]any{
		"component_count": result.Metrics.ComponentCount,
		"placed_count":    result.Metrics.PlacedCount,
		"unplaced_count":  result.Metrics.UnplacedCount,
		"fixed_count":     result.Metrics.FixedCount,
		"retry":           true,
	}
	if result.Status != placement.StatusPlaced && stage.Status == StageStatusOK {
		stage.Status = StageStatusWarning
	}
	return PlacementStageResult{Request: request, Result: result, Stage: stage}
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
	if routingStatusRank(candidate.Result.Status) != routingStatusRank(current.Result.Status) {
		return routingStatusRank(candidate.Result.Status) > routingStatusRank(current.Result.Status)
	}
	if candidate.Result.Metrics.FailedNetCount != current.Result.Metrics.FailedNetCount {
		return candidate.Result.Metrics.FailedNetCount < current.Result.Metrics.FailedNetCount
	}
	if candidate.Result.Metrics.RoutedNetCount != current.Result.Metrics.RoutedNetCount {
		return candidate.Result.Metrics.RoutedNetCount > current.Result.Metrics.RoutedNetCount
	}
	return false
}

func routingStatusRank(status routing.Status) int {
	switch status {
	case routing.StatusRouted:
		return 3
	case routing.StatusPartial:
		return 2
	default:
		return 1
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
