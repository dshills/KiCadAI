package designworkflow

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/routing"
)

const (
	minFailedNetFirstNegotiationAttempts      = 12
	maxFailedNetFirstNegotiationAttempts      = 32
	maxRouteNegotiationSearchBudgetMultiplier = 4
)

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
	baseSearchNodeLimit := normalizedRouteNegotiationSearchNodeLimit(request.Rules.MaxSearchNodes)
	summary := FinalRouteOrderNegotiationSummary{
		Attempts: 1, SelectedOrder: "baseline",
		MaximumSearchNodeLimit: baseSearchNodeLimit, SelectedSearchNodeLimit: baseSearchNodeLimit,
	}
	searchLimited := map[string]string{}
	recordSearchLimitedRoutes(searchLimited, baseline)
	if ctx != nil && ctx.Err() != nil || baseline.Metrics.FailedNetCount == 0 {
		summary.SearchLimitedNets = sortedSearchLimitedNets(searchLimited)
		summary.SelectedNetOrder = routeResultNetOrder(baseline)
		return baseline, summary
	}
	best := baseline
	promoted := map[string]string{}
	frontier := []routeNegotiationState{{request: request, result: baseline, key: routeNegotiationRequestKey(request)}}
	seen := map[string]struct{}{frontier[0].key: {}}
	attemptLimit := failedNetFirstNegotiationAttemptLimit(request)
	for len(frontier) != 0 && summary.Attempts < attemptLimit {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		stateIndex := bestRouteNegotiationStateIndex(frontier)
		state := frontier[stateIndex]
		frontier = append(frontier[:stateIndex], frontier[stateIndex+1:]...)
		for _, netName := range blockingRoutingIssueNets(state.result.Issues, request.Nets) {
			if summary.Attempts >= attemptLimit || ctx != nil && ctx.Err() != nil {
				break
			}
			key := interBlockSummaryNetKey(netName)
			candidateRequest := state.request
			candidateRequest.Nets = promoteFailedNetPriorities(candidateRequest.Nets, map[string]struct{}{key: {}})
			if routingResultHitSearchLimitForNet(state.result, netName) {
				// A promoted net with an observed search-budget exhaustion needs
				// a materially different retry. Expand the normalized search
				// budget geometrically to a fixed cap, without changing the
				// caller's direct-routing hard limit or growing unboundedly.
				currentLimit := normalizedRouteNegotiationSearchNodeLimit(candidateRequest.Rules.MaxSearchNodes)
				nextLimit := nextRouteNegotiationSearchNodeLimit(currentLimit, baseSearchNodeLimit)
				candidateRequest.Rules.MaxSearchNodes = nextLimit
				if nextLimit > currentLimit {
					summary.SearchBudgetExpansions++
				}
				summary.MaximumSearchNodeLimit = max(summary.MaximumSearchNodeLimit, nextLimit)
			}
			candidateKey := routeNegotiationRequestKey(candidateRequest)
			if _, exists := seen[candidateKey]; exists {
				continue
			}
			seen[candidateKey] = struct{}{}
			promoted[key] = netName
			candidate := route(ctx, candidateRequest)
			recordSearchLimitedRoutes(searchLimited, candidate)
			summary.Attempts++
			if routingResultBetter(candidate, best) {
				best = candidate
				summary.SelectedOrder = "failed_net_first"
				summary.SelectedSearchNodeLimit = normalizedRouteNegotiationSearchNodeLimit(candidateRequest.Rules.MaxSearchNodes)
			}
			if candidate.Metrics.FailedNetCount == 0 {
				best = candidate
				summary.SelectedSearchNodeLimit = normalizedRouteNegotiationSearchNodeLimit(candidateRequest.Rules.MaxSearchNodes)
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
	summary.SearchLimitedNets = sortedSearchLimitedNets(searchLimited)
	summary.SelectedNetOrder = routeResultNetOrder(best)
	return best, summary
}

func failedNetFirstNegotiationAttemptLimit(request routing.Request) int {
	// A sequential router can expose one new blocked net after each successful
	// promotion. Give every net one bounded opportunity while retaining the
	// established minimum for small boards and a hard ceiling for large ones.
	return min(maxFailedNetFirstNegotiationAttempts, max(minFailedNetFirstNegotiationAttempts, len(request.Nets)+1))
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
	builder.WriteString("search_nodes=")
	builder.WriteString(strconv.Itoa(request.Rules.MaxSearchNodes))
	builder.WriteByte('\x00')
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

func normalizedRouteNegotiationSearchNodeLimit(limit int) int {
	if limit <= 0 {
		return routing.DefaultRules().MaxSearchNodes
	}
	return limit
}

func nextRouteNegotiationSearchNodeLimit(current, base int) int {
	base = normalizedRouteNegotiationSearchNodeLimit(base)
	if current <= 0 {
		current = base
	}
	maximum := math.MaxInt
	if base <= math.MaxInt/maxRouteNegotiationSearchBudgetMultiplier {
		maximum = base * maxRouteNegotiationSearchBudgetMultiplier
	}
	if current >= maximum {
		return maximum
	}
	if current > math.MaxInt/2 {
		return math.MaxInt
	}
	return min(current*2, maximum)
}

func routingResultHitSearchLimitForNet(result routing.Result, netName string) bool {
	key := interBlockSummaryNetKey(netName)
	for _, route := range result.Routes {
		if interBlockSummaryNetKey(route.Net) == key {
			return route.SearchLimitHit
		}
	}
	return false
}

func recordSearchLimitedRoutes(target map[string]string, result routing.Result) {
	for _, route := range result.Routes {
		if !route.SearchLimitHit {
			continue
		}
		target[interBlockSummaryNetKey(route.Net)] = route.Net
	}
}

func sortedSearchLimitedNets(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func routeResultNetOrder(result routing.Result) []string {
	order := make([]string, 0, len(result.Routes))
	for _, route := range result.Routes {
		order = append(order, route.Net)
	}
	return order
}
