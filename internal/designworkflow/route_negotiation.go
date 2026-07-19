package designworkflow

import (
	"context"
	"slices"

	"kicadai/internal/routing"
)

const maxFailedNetFirstNegotiationAttempts = 8

// routeWithFailedNetFirstNegotiation promotes each newly blocked net above the
// order that blocked it. The bounded priority tiers let a later attempt repair
// a failure exposed by an earlier promotion instead of stopping after a single
// swap. Both explicit-PCB and generated-PCB routing use this same deterministic
// negotiation policy.
func routeWithFailedNetFirstNegotiation(ctx context.Context, request routing.Request) (routing.Result, FinalRouteOrderNegotiationSummary) {
	baseline := routing.RouteRequestContext(ctx, request)
	summary := FinalRouteOrderNegotiationSummary{Attempts: 1, SelectedOrder: "baseline"}
	if ctx != nil && ctx.Err() != nil || baseline.Metrics.FailedNetCount == 0 {
		return baseline, summary
	}
	best := baseline
	current := baseline
	currentRequest := request
	seenFailedSets := map[string]struct{}{}
	promoted := map[string]string{}
	attemptLimit := min(maxFailedNetFirstNegotiationAttempts, len(request.Nets)+1)
	for summary.Attempts < attemptLimit {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		failed := blockingRoutingIssueNets(current.Issues, request.Nets)
		if len(failed) == 0 {
			break
		}
		failedKey := ""
		for _, netName := range failed {
			failedKey += "\x00" + interBlockSummaryNetKey(netName)
			promoted[interBlockSummaryNetKey(netName)] = netName
		}
		if _, seen := seenFailedSets[failedKey]; seen {
			break
		}
		seenFailedSets[failedKey] = struct{}{}
		failedSet := make(map[string]struct{}, len(failed))
		for _, netName := range failed {
			failedSet[interBlockSummaryNetKey(netName)] = struct{}{}
		}
		// promoteFailedNetPriorities always allocates a new slice, so each tier
		// preserves the previous attempt without mutating the caller's request.
		currentRequest.Nets = promoteFailedNetPriorities(currentRequest.Nets, failedSet)
		current = routing.RouteRequestContext(ctx, currentRequest)
		summary.Attempts++
		if routingResultBetter(current, best) {
			best = current
			summary.SelectedOrder = "failed_net_first"
		}
		if current.Metrics.FailedNetCount == 0 {
			break
		}
	}
	for _, netName := range promoted {
		summary.PromotedNets = append(summary.PromotedNets, netName)
	}
	slices.Sort(summary.PromotedNets)
	return best, summary
}
