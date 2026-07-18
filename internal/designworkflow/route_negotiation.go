package designworkflow

import (
	"context"

	"kicadai/internal/routing"
)

// routeWithFailedNetFirstNegotiation retries a blocked request once with the
// failed nets strictly promoted, retaining the result only when it improves
// route completion. Both explicit-PCB and generated-PCB routing use this same
// deterministic negotiation policy.
func routeWithFailedNetFirstNegotiation(ctx context.Context, request routing.Request) (routing.Result, FinalRouteOrderNegotiationSummary) {
	baseline := routing.RouteRequestContext(ctx, request)
	summary := FinalRouteOrderNegotiationSummary{Attempts: 1, SelectedOrder: "baseline"}
	if ctx != nil && ctx.Err() != nil || baseline.Metrics.FailedNetCount == 0 {
		return baseline, summary
	}
	failed := blockingRoutingIssueNets(baseline.Issues, request.Nets)
	if len(failed) == 0 {
		return baseline, summary
	}
	failedSet := make(map[string]struct{}, len(failed))
	for _, netName := range failed {
		failedSet[interBlockSummaryNetKey(netName)] = struct{}{}
	}
	retryRequest := request
	// promoteFailedNetPriorities always allocates a new slice, so the shallow
	// struct copy cannot mutate the caller's request or its net priorities.
	retryRequest.Nets = promoteFailedNetPriorities(request.Nets, failedSet)
	retry := routing.RouteRequestContext(ctx, retryRequest)
	summary.Attempts = 2
	summary.PromotedNets = failed
	if routingResultBetter(retry, baseline) {
		summary.SelectedOrder = "failed_net_first"
		return retry, summary
	}
	return baseline, summary
}
