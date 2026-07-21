package designworkflow

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/routing"
)

const maxFailedNetFirstNegotiationAttempts = 12

type routeNegotiationState struct {
	request routing.Request
	result  routing.Result
	key     string
}

// routeWithFailedNetFirstNegotiation explores bounded, deterministic
// single-net promotions. Expanding one observed failure at a time avoids
// collapsing unrelated failed nets into one priority tier and permits a
// temporary regression when it is the only route to a better ordering.
func routeWithFailedNetFirstNegotiation(ctx context.Context, request routing.Request) (routing.Result, FinalRouteOrderNegotiationSummary) {
	return routeWithFailedNetFirstNegotiationUsing(ctx, request, routing.RouteRequestContext)
}

func routeWithFailedNetFirstNegotiationUsing(ctx context.Context, request routing.Request, route func(context.Context, routing.Request) routing.Result) (routing.Result, FinalRouteOrderNegotiationSummary) {
	baseline := route(ctx, request)
	summary := FinalRouteOrderNegotiationSummary{Attempts: 1, SelectedOrder: "baseline"}
	if ctx != nil && ctx.Err() != nil || baseline.Metrics.FailedNetCount == 0 {
		return baseline, summary
	}
	best := baseline
	promoted := map[string]string{}
	frontier := []routeNegotiationState{{request: request, result: baseline, key: routeNegotiationRequestKey(request)}}
	seen := map[string]struct{}{frontier[0].key: {}}
	for len(frontier) != 0 && summary.Attempts < maxFailedNetFirstNegotiationAttempts {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		stateIndex := bestRouteNegotiationStateIndex(frontier)
		state := frontier[stateIndex]
		frontier = append(frontier[:stateIndex], frontier[stateIndex+1:]...)
		for _, netName := range blockingRoutingIssueNets(state.result.Issues, request.Nets) {
			if summary.Attempts >= maxFailedNetFirstNegotiationAttempts || ctx != nil && ctx.Err() != nil {
				break
			}
			key := interBlockSummaryNetKey(netName)
			candidateRequest := state.request
			candidateRequest.Nets = promoteFailedNetPriorities(candidateRequest.Nets, map[string]struct{}{key: {}})
			candidateKey := routeNegotiationRequestKey(candidateRequest)
			if _, exists := seen[candidateKey]; exists {
				continue
			}
			seen[candidateKey] = struct{}{}
			promoted[key] = netName
			candidate := route(ctx, candidateRequest)
			summary.Attempts++
			if routingResultBetter(candidate, best) {
				best = candidate
				summary.SelectedOrder = "failed_net_first"
			}
			if candidate.Metrics.FailedNetCount == 0 {
				best = candidate
				frontier = nil
				break
			}
			frontier = append(frontier, routeNegotiationState{request: candidateRequest, result: candidate, key: candidateKey})
		}
	}
	for _, netName := range promoted {
		summary.PromotedNets = append(summary.PromotedNets, netName)
	}
	slices.Sort(summary.PromotedNets)
	return best, summary
}

func bestRouteNegotiationStateIndex(states []routeNegotiationState) int {
	best := 0
	for index := 1; index < len(states); index++ {
		if routingResultBetter(states[index].result, states[best].result) ||
			!routingResultBetter(states[best].result, states[index].result) && states[index].key < states[best].key {
			best = index
		}
	}
	return best
}

func routeNegotiationRequestKey(request routing.Request) string {
	var builder strings.Builder
	for _, net := range request.Nets {
		builder.WriteString(interBlockSummaryNetKey(net.Name))
		builder.WriteByte('=')
		builder.WriteString(strconv.Itoa(net.Priority))
		if net.OrderFirst {
			builder.WriteByte('!')
		}
		builder.WriteByte('\x00')
	}
	return builder.String()
}
