package routing

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/pcbrules"
	"kicadai/internal/reports"
)

const routePairContextCheckInterval = 16

// Keep fallback access strictly inside SMD copper so the emitted endpoint is
// recognized as connected after integer-unit KiCad serialization.
const smdEdgeAccessInsetRatio = 0.9

func RouteRequest(request Request) Result {
	return RouteRequestContext(context.Background(), request)
}

func RouteRequestContext(ctx context.Context, request Request) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if result, canceled := routeCanceledResult(ctx); canceled {
		return result
	}
	request = cloneRequest(request)
	NormalizeRequest(&request)
	components := componentsByNormalizedRef(request.Components)
	qualityEvidence := BuildQualityInputEvidence(request)
	result := Result{Status: StatusBlocked}
	issues := Validate(&request)
	access := BuildPadAccess(request)
	issues = append(issues, access.Issues...)
	if hasBlockingIssue(issues) {
		result.Issues = issues
		return result
	}
	plans, planIssues := PlanRoutes(request, access)
	issues = append(issues, planIssues...)
	if hasBlockingIssue(planIssues) {
		result.Issues = issues
		return result
	}
	ruleSet := toPCBRules(request.Rules, request.Strategy)
	ruleResolver := pcbrules.NewResolver(ruleSet)
	result.Metrics.NetCount = len(plans)
	failed := false
	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			result.Status = StatusBlocked
			result.Issues = append(result.Issues, routeCanceledIssue(err))
			return result
		}
		route := Route{Net: plan.Net.Name, Status: RouteStatusRouted}
		netRequest := request
		effectiveRule, ruleIssues := ResolveNetRuleWithResolver(ruleResolver, plan.Net)
		if len(ruleIssues) != 0 {
			route.Issues = append(route.Issues, ruleIssues...)
		}
		netFailed := hasBlockingIssue(ruleIssues)
		if netFailed {
			route.Status = RouteStatusFailed
		}
		netRequest.Rules = applyEffectiveRule(request.Rules, effectiveRule)
		netRequest.Rules = applyAutomaticEndpointNeckdown(netRequest.Rules, plan.Net.Role, netHasNarrowEndpoint(netRequest, plan.Net))
		searchRequest := routingSearchRequest(netRequest)
		if plan.Net.Class == "" && (plan.Net.Role == NetPower || plan.Net.Role == NetGround || plan.Net.Role == NetHighCurrent) {
			route.Issues = append(route.Issues, reports.Issue{
				Code:       reports.CodeMissingNetClass,
				Severity:   reports.SeverityWarning,
				Path:       "nets." + plan.Net.Name + ".class",
				Message:    "power or high-current net has no explicit net class",
				Nets:       []string{plan.Net.Name},
				Suggestion: "assign a net class with explicit trace, via, and clearance rules",
			})
		}
		var occupancy Occupancy
		var viaOccupancy Occupancy
		var nominalOccupancy Occupancy
		if !netFailed {
			var err error
			if searchRequest.Rules.TraceWidthMM != netRequest.Rules.TraceWidthMM {
				nominalOccupancy, _, err = buildRouteOccupancy(netRequest, plan.Net.Name)
			} else {
				occupancy, viaOccupancy, err = buildRouteOccupancy(searchRequest, plan.Net.Name)
				nominalOccupancy = occupancy
			}
			if err != nil {
				if issue, ok := reports.IssueFromError(err); ok {
					route.Issues = append(route.Issues, issue)
				} else {
					route.Issues = append(route.Issues, reports.Issue{
						Code:     reports.CodeValidationFailed,
						Severity: reports.SeverityBlocked,
						Message:  err.Error(),
						Nets:     []string{plan.Net.Name},
					})
				}
				netFailed = true
				failed = true
			}
			if !netFailed && searchRequest.Rules.TraceWidthMM != netRequest.Rules.TraceWidthMM {
				occupancy, viaOccupancy, err = buildRouteOccupancy(searchRequest, plan.Net.Name)
				if err != nil {
					route.Issues = append(route.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: err.Error(), Nets: []string{plan.Net.Name}})
					netFailed = true
					failed = true
				}
			}
		} else {
			failed = true
		}
		existingStart := len(request.Existing)
		netSegmentCount := 0
		netViaCount := 0
		netLengthMM := 0.0
		var pendingNeckdownTrunkPair *struct {
			index int
			pair  EndpointPair
		}
		var fallbackRequest Request
		var fallbackOccupancy Occupancy
		var fallbackViaOccupancy Occupancy
		fallbackReady := false
		netAccess := clonePadAccessPoints(access)
		pairs := plan.Pairs
		constrainedEndpointAccess := searchRequest.Strategy.NetOrder == NetOrderConstrainedEndpointAccessV1
		if constrainedEndpointAccess {
			pairs = prioritizeCrowdedSMDPadPairs(pairs, searchRequest)
		}
		for pairIndex, pair := range pairs {
			if pairIndex%routePairContextCheckInterval == 0 {
				if err := ctx.Err(); err != nil {
					result.Status = StatusBlocked
					result.Issues = append(result.Issues, routeCanceledIssue(err))
					return result
				}
			}
			if netFailed {
				break
			}
			pairAccess := netAccess
			var forcedEndpointVias map[endpointID]float64
			var crowdedAccessAlternates []PadAccess
			if constrainedEndpointAccess {
				pairAccess, forcedEndpointVias = applyCrowdedSMDPadViaAccess(netAccess, searchRequest, components, []Endpoint{pair.From, pair.To})
				if len(forcedEndpointVias) != 0 {
					candidates := crowdedSMDPadViaAccessAttempts(pairAccess, forcedEndpointVias)
					pairAccess = candidates[0]
					crowdedAccessAlternates = candidates[1:]
				}
			}
			pairAccess = filterPhysicalEndpointAccess(pairAccess, searchRequest, plan.Net.Name, []Endpoint{pair.From, pair.To})
			pairRequest := searchRequest
			pairViaRules := netRequest.Rules
			pairViaOccupancy := viaOccupancy
			pairUsedCrowdedVias := len(forcedEndpointVias) != 0
			if pairUsedCrowdedVias {
				pairRequest.Rules = crowdedEndpointViaRules(pairRequest.Rules, forcedEndpointVias)
				pairViaRules = crowdedEndpointViaRules(pairViaRules, forcedEndpointVias)
				if candidate, err := BuildViaOccupancy(pairRequest, plan.Net.Name); err == nil {
					pairViaOccupancy = candidate
				}
			}
			path, routeIssues := routePairPathWithForcedEndpointVias(ctx, pairRequest, pairAccess, occupancy, pairViaOccupancy, plan.Net.Name, pair, forcedEndpointVias)
			route.SearchNodes += path.SearchNodes
			result.Metrics.SearchNodes += path.SearchNodes
			if path.SearchLimitHit {
				route.SearchLimitHit = true
				result.Metrics.MaxSearchNodesHit = true
			}
			if len(routeIssues) != 0 {
				for _, alternateAccess := range crowdedAccessAlternates {
					alternateAccess = filterPhysicalEndpointAccess(alternateAccess, pairRequest, plan.Net.Name, []Endpoint{pair.From, pair.To})
					alternatePath, alternateIssues := routePairPathWithForcedEndpointVias(ctx, pairRequest, alternateAccess, occupancy, pairViaOccupancy, plan.Net.Name, pair, forcedEndpointVias)
					route.SearchNodes += alternatePath.SearchNodes
					result.Metrics.SearchNodes += alternatePath.SearchNodes
					if alternatePath.SearchLimitHit {
						route.SearchLimitHit = true
						result.Metrics.MaxSearchNodesHit = true
					}
					path = alternatePath
					routeIssues = alternateIssues
					if len(alternateIssues) == 0 {
						pairAccess = alternateAccess
						break
					}
				}
			}
			if len(routeIssues) == 0 {
				netAccess = pairAccess
			}
			// Dense neighboring copper can block a pad center on any board.
			// Retry from deterministic physical pad-edge access before changing
			// width or declaring the endpoint inaccessible.
			if len(routeIssues) != 0 {
				edgeAccess := expandSMDPadEdgeAccess(netAccess, searchRequest, []Endpoint{pair.From, pair.To})
				edgeAccess = filterPhysicalEndpointAccess(edgeAccess, searchRequest, plan.Net.Name, []Endpoint{pair.From, pair.To})
				edgePath, edgeIssues := routePairPath(ctx, searchRequest, edgeAccess, occupancy, viaOccupancy, plan.Net.Name, pair)
				route.SearchNodes += edgePath.SearchNodes
				result.Metrics.SearchNodes += edgePath.SearchNodes
				if edgePath.SearchLimitHit {
					route.SearchLimitHit = true
					result.Metrics.MaxSearchNodesHit = true
				}
				// Keep the expanded access for the neckdown retry as well. Once a
				// route succeeds, endpoint validation narrows each endpoint back to
				// the single physical access point actually used.
				netAccess = edgeAccess
				path = edgePath
				routeIssues = edgeIssues
				if len(edgeIssues) == 0 {
					pairUsedCrowdedVias = false
				}
			}
			// A two-terminal SMD pad can be sealed by the opposite pad on the
			// same package even after all four edge-access points are tried.
			// Retry only after ordinary access fails, with a deterministic
			// outward dogbone that moves away from the opposite terminal.
			if len(routeIssues) != 0 && constrainedEndpointAccess {
				dogboneAccess, dogboneVias := applyTwoTerminalSMDPadViaAccess(netAccess, searchRequest, components, []Endpoint{pair.From, pair.To})
				if len(dogboneVias) != 0 {
					dogboneRules := crowdedEndpointViaRules(searchRequest.Rules, dogboneVias)
					dogboneRequest := searchRequest
					dogboneRequest.Rules = dogboneRules
					dogboneViaOccupancy := viaOccupancy
					if candidate, err := BuildViaOccupancy(dogboneRequest, plan.Net.Name); err == nil {
						dogboneViaOccupancy = candidate
					}
					for _, attemptAccess := range crowdedSMDPadViaAccessAttempts(dogboneAccess, dogboneVias) {
						attemptAccess = filterPhysicalEndpointAccess(attemptAccess, dogboneRequest, plan.Net.Name, []Endpoint{pair.From, pair.To})
						dogbonePath, dogboneIssues := routePairPathWithForcedEndpointVias(
							ctx, dogboneRequest, attemptAccess, occupancy, dogboneViaOccupancy, plan.Net.Name, pair, dogboneVias,
						)
						route.SearchNodes += dogbonePath.SearchNodes
						result.Metrics.SearchNodes += dogbonePath.SearchNodes
						if dogbonePath.SearchLimitHit {
							route.SearchLimitHit = true
							result.Metrics.MaxSearchNodesHit = true
						}
						path = dogbonePath
						routeIssues = dogboneIssues
						if len(dogboneIssues) == 0 {
							netAccess = attemptAccess
							forcedEndpointVias = dogboneVias
							pairViaRules = dogboneRules
							pairViaOccupancy = dogboneViaOccupancy
							pairUsedCrowdedVias = true
							break
						}
					}
				}
			}
			neckdownWidthMM := netRequest.Rules.NeckdownWidthMM
			neckdownLengthMM := netRequest.Rules.NeckdownLengthMM
			if len(routeIssues) != 0 && neckdownWidthMM == 0 {
				candidate, ok := endpointNeckdownFallbackRequest(netRequest, netRequest.Rules, plan.Net.Role, len(routableLayerNames(netRequest.Board.Layers)) > 2)
				if ok && ctx.Err() == nil {
					if !fallbackReady {
						fallbackRequest = candidate
						var err error
						fallbackOccupancy, err = BuildOccupancy(fallbackRequest, plan.Net.Name)
						if err == nil {
							fallbackViaOccupancy = viaOccupancy
							fallbackReady = true
						}
					}
					if fallbackReady {
						fallbackAccess := filterPhysicalEndpointAccess(netAccess, fallbackRequest, plan.Net.Name, []Endpoint{pair.From, pair.To})
						fallbackPath, fallbackIssues := routePairPath(ctx, fallbackRequest, fallbackAccess, fallbackOccupancy, fallbackViaOccupancy, plan.Net.Name, pair)
						route.SearchNodes += fallbackPath.SearchNodes
						result.Metrics.SearchNodes += fallbackPath.SearchNodes
						if len(fallbackIssues) == 0 {
							path = fallbackPath
							netAccess = fallbackAccess
							pairUsedCrowdedVias = false
							neckdownWidthMM = fallbackRequest.Rules.TraceWidthMM
							neckdownLengthMM = pcbrules.DefaultPowerNeckdownLengthMM
						}
						routeIssues = fallbackIssues
					}
				}
			}
			var segments []Segment
			var metrics Metrics
			if len(routeIssues) == 0 {
				segments, metrics = BuildSegmentsFromPathWithNeckdown(path, netRequest.Rules.TraceWidthMM, neckdownWidthMM, neckdownLengthMM)
				if segmentsUseNeckdown(segments, netRequest.Rules.TraceWidthMM) && !nominalSegmentsClearOccupancy(segments, netRequest.Rules.TraceWidthMM, nominalOccupancy, netRequest.Board.Layers) {
					var extended bool
					segments, metrics, extended = extendEndpointNeckdownToClearTrunk(path, netRequest.Rules.TraceWidthMM, neckdownWidthMM, neckdownLengthMM, nominalOccupancy, netRequest.Board.Layers, route.Segments)
					if !extended {
						// Do not make a multi-endpoint net's result depend on branch
						// order. A short or obstructed first branch may legitimately be
						// all neckdown when a later branch establishes the full-width
						// trunk. Retain the safe narrow branch provisionally and verify
						// the completed net contains nominal-width copper below.
						var provisional bool
						segments, metrics, provisional = endpointNeckdownAwaitingNetTrunk(path, netRequest.Rules.TraceWidthMM, neckdownWidthMM, nominalOccupancy, netRequest.Board.Layers)
						if provisional {
							if pendingNeckdownTrunkPair == nil {
								pendingNeckdownTrunkPair = &struct {
									index int
									pair  EndpointPair
								}{index: pairIndex, pair: pair}
							}
						} else {
							routeIssues = []reports.Issue{endpointNeckdownTrunkIssue(plan.Net.Name, pairIndex, pair)}
						}
					}
				}
				if len(routeIssues) == 0 && (!pinPathEndpointAccess(&netAccess, path, pair.From, 0) || !pinPathEndpointAccess(&netAccess, path, pair.To, len(path.Points)-1)) {
					routeIssues = []reports.Issue{routeEndpointAccessIssue(plan.Net.Name, pairIndex, pair)}
				}
				if len(routeIssues) == 0 {
					segments = connectFallbackSMDEndpointsToCenters(segments, netAccess, pair)
					metrics.TotalLengthMM = segmentLengthTotal(segments)
				}
			}
			if len(routeIssues) != 0 {
				route.Issues = append(route.Issues, routeIssues...)
				netFailed = true
				failed = true
				break
			}
			if !pairUsedCrowdedVias {
				pairViaRules = netRequest.Rules
			}
			vias := BuildViasFromPath(path, pairViaRules)
			route.Segments = append(route.Segments, segments...)
			route.Vias = append(route.Vias, vias...)
			netSegmentCount += len(segments)
			netViaCount += len(vias)
			netLengthMM += metrics.TotalLengthMM
			request.Existing = append(request.Existing, existingCopperForSegments(segments)...)
			request.Existing = append(request.Existing, existingCopperForVias(vias, request.Board.Layers)...)
		}
		if !netFailed && pendingNeckdownTrunkPair != nil && !segmentsContainNominalWidth(route.Segments, netRequest.Rules.TraceWidthMM) {
			route.Issues = append(route.Issues, endpointNeckdownTrunkIssue(plan.Net.Name, pendingNeckdownTrunkPair.index, pendingNeckdownTrunkPair.pair))
			netFailed = true
			failed = true
		}
		if !netFailed {
			route.Segments = pruneConnectedSameLayerSegmentCycles(request, route, access)
			netSegmentCount = len(route.Segments)
			netLengthMM = segmentLengthTotal(route.Segments)
			request.Existing = request.Existing[:existingStart]
			request.Existing = append(request.Existing, existingCopperForSegments(route.Segments)...)
			request.Existing = append(request.Existing, existingCopperForVias(route.Vias, request.Board.Layers)...)
		}
		if netFailed || hasBlockingIssue(route.Issues) {
			request.Existing = request.Existing[:existingStart]
			failed = true
			route.Status = RouteStatusFailed
			result.Metrics.FailedNetCount++
			result.Routes = append(result.Routes, route)
			if !request.Strategy.AllowPartial {
				result.Status = StatusBlocked
				result.Issues = append(issues, collectRouteIssues(result.Routes)...)
				quality := BuildQualityReportWithEvidence(request, result, qualityEvidence)
				result.Quality = &quality
				return result
			}
			continue
		}
		route.Issues = append(route.Issues, lengthPolicyIssues(plan.Net.Name, effectiveRule, route)...)
		if hasBlockingIssue(route.Issues) {
			request.Existing = request.Existing[:existingStart]
			route.Status = RouteStatusFailed
			result.Metrics.FailedNetCount++
			result.Routes = append(result.Routes, route)
			if !request.Strategy.AllowPartial {
				result.Status = StatusBlocked
				result.Issues = append(issues, collectRouteIssues(result.Routes)...)
				quality := BuildQualityReportWithEvidence(request, result, qualityEvidence)
				result.Quality = &quality
				return result
			}
			failed = true
			continue
		}
		result.Metrics.SegmentCount += netSegmentCount
		result.Metrics.ViaCount += netViaCount
		result.Metrics.TotalLengthMM = roundMM(result.Metrics.TotalLengthMM + netLengthMM)
		result.Metrics.RoutedNetCount++
		result.Routes = append(result.Routes, route)
	}
	result.Issues = append(issues, collectRouteIssues(result.Routes)...)
	if failed {
		result.Status = StatusPartial
	} else {
		result.Status = StatusRouted
	}
	validation := ValidateResult(request, result)
	if len(validation.Issues) != 0 {
		result.Issues = append(result.Issues, validation.Issues...)
		if result.Status == StatusRouted {
			result.Status = StatusBlocked
		}
	}
	operations, operationIssues := OperationsFromResultWithIssues(result)
	result.Operations = operations
	if len(operationIssues) != 0 {
		seenIssues := map[issueKey]struct{}{}
		result.Issues = appendUniqueIssues(nil, result.Issues, seenIssues)
		result.Issues = appendUniqueIssues(result.Issues, operationIssues, seenIssues)
		if result.Status == StatusRouted && reports.HasBlockingIssue(operationIssues) {
			result.Status = StatusBlocked
		}
	}
	quality := BuildQualityReportWithEvidence(request, result, qualityEvidence)
	result.Quality = &quality
	return result
}

func connectFallbackSMDEndpointsToCenters(segments []Segment, access PadAccess, pair EndpointPair) []Segment {
	if len(segments) == 0 {
		return segments
	}
	centerConnection := func(endpoint Endpoint, segment Segment, start bool) (Segment, bool) {
		points, ok := AccessPointsForEndpoint(access, endpoint)
		if !ok || len(points) != 1 || points[0].SearchPoint == nil {
			return Segment{}, false
		}
		pad, ok := access.Pads[endpointKey(endpoint.Ref, endpoint.Pin)]
		if !ok || pad.Type != PadSMD {
			return Segment{}, false
		}
		connection := segment
		if start {
			connection.Start = pad.Position
			connection.End = segment.Start
		} else {
			connection.Start = segment.End
			connection.End = pad.Position
		}
		return connection, roundPoint(connection.Start) != roundPoint(connection.End)
	}
	connected := make([]Segment, 0, len(segments)+2)
	if connection, ok := centerConnection(pair.From, segments[0], true); ok {
		connected = append(connected, connection)
	}
	connected = append(connected, segments...)
	if connection, ok := centerConnection(pair.To, segments[len(segments)-1], false); ok {
		connected = append(connected, connection)
	}
	return connected
}

func segmentLengthTotal(segments []Segment) float64 {
	total := 0.0
	for _, segment := range segments {
		total += pointDistance(segment.Start, segment.End)
	}
	return roundMM(total)
}

type routeSegmentVertex struct {
	Layer    string
	XMM, YMM float64
}

func pruneConnectedSameLayerSegmentCycles(request Request, route Route, access PadAccess) []Segment {
	segments := append([]Segment(nil), route.Segments...)
	for {
		candidates := sameLayerCycleClosingIndexes(segments)
		removed := false
		for index := len(candidates) - 1; index >= 0; index-- {
			candidateSegments := removeSegmentIndexes(segments, []int{candidates[index]})
			candidateRoute := route
			candidateRoute.Segments = candidateSegments
			if routeEndpointsConnected(request, candidateRoute, access) {
				segments = candidateSegments
				removed = true
				break
			}
		}
		if !removed {
			return pruneRedundantSameLayerSegmentLeaves(request, route, access, segments)
		}
	}
}

func pruneRedundantSameLayerSegmentLeaves(request Request, route Route, access PadAccess, segments []Segment) []Segment {
	for {
		degrees := make(map[routeSegmentVertex]int, len(segments)*2)
		for _, segment := range segments {
			degrees[routeSegmentVertex{Layer: segment.Layer, XMM: segment.Start.XMM, YMM: segment.Start.YMM}]++
			degrees[routeSegmentVertex{Layer: segment.Layer, XMM: segment.End.XMM, YMM: segment.End.YMM}]++
		}
		removed := false
		for index := len(segments) - 1; index >= 0; index-- {
			segment := segments[index]
			start := routeSegmentVertex{Layer: segment.Layer, XMM: segment.Start.XMM, YMM: segment.Start.YMM}
			end := routeSegmentVertex{Layer: segment.Layer, XMM: segment.End.XMM, YMM: segment.End.YMM}
			startLeaf := degrees[start] == 1 && !routePointHasVia(route.Vias, segment.Start)
			endLeaf := degrees[end] == 1 && !routePointHasVia(route.Vias, segment.End)
			if !startLeaf && !endLeaf {
				continue
			}
			candidateSegments := removeSegmentIndexes(segments, []int{index})
			candidateRoute := route
			candidateRoute.Segments = candidateSegments
			if routeEndpointsConnected(request, candidateRoute, access) {
				segments = candidateSegments
				removed = true
				break
			}
		}
		if !removed {
			return segments
		}
	}
}

func routePointHasVia(vias []Via, point Point) bool {
	for _, via := range vias {
		if roundPoint(via.At) == roundPoint(point) {
			return true
		}
	}
	return false
}

func sameLayerCycleClosingIndexes(segments []Segment) []int {
	parent := map[routeSegmentVertex]routeSegmentVertex{}
	find := func(vertex routeSegmentVertex) routeSegmentVertex {
		root, ok := parent[vertex]
		if !ok {
			parent[vertex] = vertex
			return vertex
		}
		for root != parent[root] {
			root = parent[root]
		}
		for vertex != root {
			next := parent[vertex]
			parent[vertex] = root
			vertex = next
		}
		return root
	}
	vertexFor := func(point Point, layer string) routeSegmentVertex {
		return routeSegmentVertex{Layer: normalizeLayer(layer), XMM: roundMM(point.XMM), YMM: roundMM(point.YMM)}
	}
	closing := make([]int, 0)
	for index, segment := range segments {
		startRoot := find(vertexFor(segment.Start, segment.Layer))
		endRoot := find(vertexFor(segment.End, segment.Layer))
		if startRoot == endRoot {
			closing = append(closing, index)
			continue
		}
		parent[endRoot] = startRoot
	}
	return closing
}

func removeSegmentIndexes(segments []Segment, indexes []int) []Segment {
	if len(indexes) == 0 {
		return append([]Segment(nil), segments...)
	}
	removed := map[int]struct{}{}
	for _, index := range indexes {
		removed[index] = struct{}{}
	}
	kept := make([]Segment, 0, len(segments)-len(removed))
	for index, segment := range segments {
		if _, ok := removed[index]; !ok {
			kept = append(kept, segment)
		}
	}
	return kept
}

func clonePadAccessPoints(access PadAccess) PadAccess {
	cloned := access
	cloned.AccessPoints = make(map[endpointID][]AccessPoint, len(access.AccessPoints))
	for endpoint, points := range access.AccessPoints {
		cloned.AccessPoints[endpoint] = append([]AccessPoint(nil), points...)
	}
	return cloned
}

func expandSMDPadEdgeAccess(access PadAccess, request Request, endpoints []Endpoint) PadAccess {
	expanded := clonePadAccessPoints(access)
	wanted := make(map[endpointID]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		wanted[endpointKey(endpoint.Ref, endpoint.Pin)] = struct{}{}
	}
	routableLayers := routableLayerNames(request.Board.Layers)
	for _, component := range request.Components {
		for _, pad := range component.Pads {
			key := endpointKey(component.Ref, pad.Name)
			if _, ok := wanted[key]; !ok || pad.Type != PadSMD {
				continue
			}
			center := absolutePadPoint(component, pad.Position)
			physicalOffsets := []Point{
				{XMM: -pad.Size.WidthMM * smdEdgeAccessInsetRatio / 2},
				{XMM: pad.Size.WidthMM * smdEdgeAccessInsetRatio / 2},
				{YMM: -pad.Size.HeightMM * smdEdgeAccessInsetRatio / 2},
				{YMM: pad.Size.HeightMM * smdEdgeAccessInsetRatio / 2},
			}
			searchOffsets := []Point{
				{XMM: -pad.Size.WidthMM / 2},
				{XMM: pad.Size.WidthMM / 2},
				{YMM: -pad.Size.HeightMM / 2},
				{YMM: pad.Size.HeightMM / 2},
			}
			accessRotationDeg := component.Position.RotationDeg + pad.RotationDeg
			for _, layer := range padAccessLayers(pad, routableLayers) {
				for index, offset := range physicalOffsets {
					physicalX, physicalY := kicadfiles.RotateBoardLocalXY(offset.XMM, offset.YMM, accessRotationDeg)
					physicalPoint := Point{XMM: center.XMM + physicalX, YMM: center.YMM + physicalY}
					searchOffset := searchOffsets[index]
					searchX, searchY := kicadfiles.RotateBoardLocalXY(searchOffset.XMM, searchOffset.YMM, accessRotationDeg)
					searchPoint := Point{XMM: center.XMM + searchX, YMM: center.YMM + searchY}
					expanded.AccessPoints[key] = append(expanded.AccessPoints[key], AccessPoint{
						Endpoint:    Endpoint{Ref: component.Ref, Pin: pad.Name},
						Point:       physicalPoint,
						SearchPoint: &searchPoint,
						Layer:       layer,
					})
				}
			}
		}
	}
	return expanded
}

func smdPadEscapeMargin(rules Rules) float64 {
	defaults := DefaultRules()
	gridMM := rules.GridMM
	if gridMM <= 0 || math.IsNaN(gridMM) || math.IsInf(gridMM, 0) {
		gridMM = defaults.GridMM
	}
	traceWidthMM := rules.TraceWidthMM
	if traceWidthMM <= 0 || math.IsNaN(traceWidthMM) || math.IsInf(traceWidthMM, 0) {
		traceWidthMM = defaults.TraceWidthMM
	}
	clearanceMM := rules.ClearanceMM
	if clearanceMM <= 0 || math.IsNaN(clearanceMM) || math.IsInf(clearanceMM, 0) {
		clearanceMM = defaults.ClearanceMM
	}
	viaDiameterMM := rules.ViaDiameterMM
	if viaDiameterMM <= 0 || math.IsNaN(viaDiameterMM) || math.IsInf(viaDiameterMM, 0) {
		viaDiameterMM = defaults.ViaDiameterMM
	}
	viaClearanceMM := rules.ViaClearanceMM
	if viaClearanceMM <= 0 || math.IsNaN(viaClearanceMM) || math.IsInf(viaClearanceMM, 0) {
		viaClearanceMM = defaults.ViaClearanceMM
	}
	return max(gridMM, clearanceMM+traceWidthMM/2, viaClearanceMM+viaDiameterMM/2)
}

type crowdedSMDPadSide struct {
	axisX bool
	sign  float64
}

func applyCrowdedSMDPadViaAccess(access PadAccess, request Request, components map[string]Component, endpoints []Endpoint) (PadAccess, map[endpointID]float64) {
	if len(routableLayerNames(request.Board.Layers)) < 2 || request.Rules.MaxViasPerNet < 2 {
		return access, nil
	}
	adjusted := clonePadAccessPoints(access)
	forced := map[endpointID]float64{}
	for _, endpoint := range endpoints {
		component, found := components[normalizedEndpointPart(endpoint.Ref)]
		if !found {
			continue
		}
		for _, pad := range component.Pads {
			if !strings.EqualFold(strings.TrimSpace(pad.Name), strings.TrimSpace(endpoint.Pin)) {
				continue
			}
			points, viaDiameterMM, ok := crowdedSMDPadViaAccessPoints(component, pad, request)
			if !ok {
				continue
			}
			key := endpointKey(endpoint.Ref, endpoint.Pin)
			adjusted.AccessPoints[key] = points
			forced[key] = viaDiameterMM
		}
	}
	if len(forced) == 0 {
		return access, nil
	}
	return adjusted, forced
}

func applyTwoTerminalSMDPadViaAccess(access PadAccess, request Request, components map[string]Component, endpoints []Endpoint) (PadAccess, map[endpointID]float64) {
	if len(routableLayerNames(request.Board.Layers)) < 2 || request.Rules.MaxViasPerNet < 1 {
		return access, nil
	}
	adjusted := clonePadAccessPoints(access)
	forced := map[endpointID]float64{}
	for _, endpoint := range endpoints {
		component, found := components[normalizedEndpointPart(endpoint.Ref)]
		if !found {
			continue
		}
		for _, pad := range component.Pads {
			if !strings.EqualFold(strings.TrimSpace(pad.Name), strings.TrimSpace(endpoint.Pin)) {
				continue
			}
			points, viaDiameterMM, ok := twoTerminalSMDPadViaAccessPoints(component, pad, request)
			if !ok {
				continue
			}
			key := endpointKey(endpoint.Ref, endpoint.Pin)
			adjusted.AccessPoints[key] = points
			forced[key] = viaDiameterMM
		}
	}
	if len(forced) == 0 {
		return access, nil
	}
	return adjusted, forced
}

func componentsByNormalizedRef(components []Component) map[string]Component {
	indexed := make(map[string]Component, len(components))
	for _, component := range components {
		indexed[normalizedEndpointPart(component.Ref)] = component
	}
	return indexed
}

func normalizedEndpointPart(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func twoTerminalSMDPadViaAccessPoints(component Component, pad Pad, request Request) ([]AccessPoint, float64, bool) {
	if pad.Type != PadSMD || strings.TrimSpace(pad.Net) == "" {
		return nil, 0, false
	}
	oppositeNet := ""
	oppositePosition := Point{}
	oppositePadCount := 0
	for _, candidate := range component.Pads {
		if strings.TrimSpace(candidate.Net) == "" || sameOccupancyNet(candidate.Net, pad.Net) {
			continue
		}
		if oppositeNet != "" && !sameOccupancyNet(candidate.Net, oppositeNet) {
			return nil, 0, false
		}
		if candidate.Type != PadSMD {
			return nil, 0, false
		}
		oppositeNet = candidate.Net
		oppositePosition.XMM += candidate.Position.XMM
		oppositePosition.YMM += candidate.Position.YMM
		oppositePadCount++
	}
	if oppositePadCount == 0 {
		return nil, 0, false
	}
	oppositePosition.XMM /= float64(oppositePadCount)
	oppositePosition.YMM /= float64(oppositePadCount)
	dx := pad.Position.XMM - oppositePosition.XMM
	dy := pad.Position.YMM - oppositePosition.YMM
	if math.Abs(dx) <= distanceEpsilon && math.Abs(dy) <= distanceEpsilon {
		return nil, 0, false
	}
	axisX := math.Abs(dx) >= math.Abs(dy)
	sign := math.Copysign(1, dy)
	halfSpanMM := pad.Size.HeightMM / 2
	if axisX {
		sign = math.Copysign(1, dx)
		halfSpanMM = pad.Size.WidthMM / 2
	}
	if halfSpanMM <= 0 {
		return nil, 0, false
	}
	viaDiameterMM := request.Rules.ViaDiameterMM
	if viaDiameterMM <= 0 {
		viaDiameterMM = DefaultRules().ViaDiameterMM
	}
	if request.Rules.TraceWidthMM > 0 {
		viaDiameterMM = min(viaDiameterMM, 2*request.Rules.TraceWidthMM)
	}
	if viaDiameterMM <= request.Rules.TraceWidthMM+distanceEpsilon {
		return nil, 0, false
	}
	escapeDistanceMM := max(request.Rules.GridMM, request.Rules.ViaClearanceMM+viaDiameterMM/2)
	physicalOffset := Point{}
	searchOffset := Point{}
	if axisX {
		physicalOffset.XMM = sign * halfSpanMM * smdEdgeAccessInsetRatio
		searchOffset.XMM = sign * (halfSpanMM + escapeDistanceMM)
	} else {
		physicalOffset.YMM = sign * halfSpanMM * smdEdgeAccessInsetRatio
		searchOffset.YMM = sign * (halfSpanMM + escapeDistanceMM)
	}
	physicalX, physicalY := kicadfiles.RotateBoardLocalXY(physicalOffset.XMM, physicalOffset.YMM, component.Position.RotationDeg)
	searchX, searchY := kicadfiles.RotateBoardLocalXY(searchOffset.XMM, searchOffset.YMM, component.Position.RotationDeg)
	center := absolutePadPoint(component, pad.Position)
	grid := NewGrid(Point{}, request.Rules.GridMM)
	searchPoint := grid.ToPoint(grid.ToGrid(Point{XMM: center.XMM + searchX, YMM: center.YMM + searchY}, 0))
	layers := padAccessLayers(pad, routableLayerNames(request.Board.Layers))
	if len(layers) == 0 {
		return nil, 0, false
	}
	return []AccessPoint{{
		Endpoint: Endpoint{Ref: component.Ref, Pin: pad.Name},
		Point: Point{
			XMM: center.XMM + physicalX,
			YMM: center.YMM + physicalY,
		},
		SearchPoint: &searchPoint,
		Layer:       layers[0],
	}}, viaDiameterMM, true
}

func crowdedSMDPadViaAccessAttempts(access PadAccess, forced map[endpointID]float64) []PadAccess {
	keys := make([]endpointID, 0, len(forced))
	for key := range forced {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return endpointLess(
			Endpoint{Ref: keys[i].Ref, Pin: keys[i].Pin},
			Endpoint{Ref: keys[j].Ref, Pin: keys[j].Pin},
		)
	})
	alternateKeys := make([]endpointID, 0, len(keys))
	for _, key := range keys {
		if len(access.AccessPoints[key]) > 1 {
			alternateKeys = append(alternateKeys, key)
		}
	}
	// Route planning supplies one endpoint pair, so there are normally at most
	// two alternate-bearing keys. Retain that hard bound defensively if this
	// helper is reused with a broader endpoint set.
	if len(alternateKeys) > 2 {
		alternateKeys = alternateKeys[:2]
	}
	attemptCount := 1 << len(alternateKeys)
	attempts := make([]PadAccess, 0, attemptCount)
	for mask := 0; mask < attemptCount; mask++ {
		candidate := clonePadAccessPoints(access)
		for _, key := range keys {
			points := access.AccessPoints[key]
			if len(points) == 0 {
				continue
			}
			selected := 0
			for alternateIndex, alternateKey := range alternateKeys {
				if key == alternateKey && mask&(1<<alternateIndex) != 0 {
					selected = 1
					break
				}
			}
			candidate.AccessPoints[key] = []AccessPoint{points[selected]}
		}
		attempts = append(attempts, candidate)
	}
	return attempts
}

func prioritizeCrowdedSMDPadPairs(pairs []EndpointPair, request Request) []EndpointPair {
	if len(pairs) < 2 || len(routableLayerNames(request.Board.Layers)) < 2 || request.Rules.MaxViasPerNet < 2 {
		return pairs
	}
	crowded := map[endpointID]bool{}
	for _, component := range request.Components {
		for _, pad := range component.Pads {
			if _, _, ok := crowdedSMDPadViaAccessPoints(component, pad, request); ok {
				crowded[endpointKey(component.Ref, pad.Name)] = true
			}
		}
	}
	if len(crowded) == 0 {
		return pairs
	}
	rank := func(pair EndpointPair) int {
		rank := 0
		if crowded[endpointKey(pair.From.Ref, pair.From.Pin)] {
			rank++
		}
		if crowded[endpointKey(pair.To.Ref, pair.To.Pin)] {
			rank++
		}
		return rank
	}
	prioritized := append([]EndpointPair(nil), pairs...)
	sort.SliceStable(prioritized, func(i, j int) bool {
		return rank(prioritized[i]) > rank(prioritized[j])
	})
	return prioritized
}

func crowdedSMDPadViaAccessPoints(component Component, pad Pad, request Request) ([]AccessPoint, float64, bool) {
	clearances := newClearancePolicy(request)
	side, rawAxisX, ok := crowdedSMDPadSideFor(pad)
	if !ok {
		return nil, 0, false
	}
	accessPitchMM := 2*request.Rules.GridMM + request.Rules.TraceWidthMM
	if accessPitchMM <= 0 || min(pad.Size.WidthMM, pad.Size.HeightMM) > accessPitchMM+distanceEpsilon {
		return nil, 0, false
	}
	viaAccessPitchMM := request.Rules.ViaDiameterMM + request.Rules.ViaClearanceMM
	if viaAccessPitchMM <= 0 {
		defaults := DefaultRules()
		viaAccessPitchMM = defaults.ViaDiameterMM + defaults.ViaClearanceMM
	}
	type peer struct {
		pad   Pad
		along float64
	}
	var peers []peer
	for _, candidate := range component.Pads {
		candidateSide, _, candidateOK := crowdedSMDPadSideFor(candidate)
		if !candidateOK || candidateSide != side || min(candidate.Size.WidthMM, candidate.Size.HeightMM) > accessPitchMM+distanceEpsilon {
			continue
		}
		along := candidate.Position.XMM
		if side.axisX {
			along = candidate.Position.YMM
		}
		peers = append(peers, peer{pad: candidate, along: along})
	}
	sort.SliceStable(peers, func(i, j int) bool {
		if !distanceEqual(peers[i].along, peers[j].along) {
			return peers[i].along < peers[j].along
		}
		return endpointLess(Endpoint{Ref: component.Ref, Pin: peers[i].pad.Name}, Endpoint{Ref: component.Ref, Pin: peers[j].pad.Name})
	})
	index := -1
	adjacentForeign := false
	for peerIndex, candidate := range peers {
		if strings.EqualFold(strings.TrimSpace(candidate.pad.Name), strings.TrimSpace(pad.Name)) {
			index = peerIndex
			continue
		}
		if sameOccupancyNet(candidate.pad.Net, pad.Net) {
			continue
		}
		along := pad.Position.XMM
		if side.axisX {
			along = pad.Position.YMM
		}
		if math.Abs(candidate.along-along) <= viaAccessPitchMM+distanceEpsilon {
			adjacentForeign = true
		}
	}
	if index < 0 || !adjacentForeign {
		return nil, 0, false
	}
	stagger := index
	if side.sign < 0 {
		stagger++
	}
	stagger %= 2
	halfSpanMM := pad.Size.HeightMM / 2
	edgeOffset := Point{YMM: side.sign * halfSpanMM}
	if rawAxisX {
		halfSpanMM = pad.Size.WidthMM / 2
		edgeOffset = Point{XMM: side.sign * halfSpanMM}
	}
	rotationDeg := component.Position.RotationDeg + pad.RotationDeg
	edgeX, edgeY := kicadfiles.RotateBoardLocalXY(edgeOffset.XMM, edgeOffset.YMM, rotationDeg)
	center := absolutePadPoint(component, pad.Position)
	grid := NewGrid(Point{}, request.Rules.GridMM)
	layers := padAccessLayers(pad, routableLayerNames(request.Board.Layers))
	if len(layers) == 0 {
		return nil, 0, false
	}
	columns := []int{stagger, 1 - stagger}
	points := make([]AccessPoint, 0, len(columns))
	minViaDiameterMM := 0.0
	for _, column := range columns {
		viaDiameterMM := request.Rules.ViaDiameterMM
		if request.Rules.TraceWidthMM > 0 {
			viaDiameterMM = min(viaDiameterMM, 2*request.Rules.TraceWidthMM)
		}
		escapeDistanceMM := max(request.Rules.GridMM, request.Rules.ViaClearanceMM+viaDiameterMM/2) +
			float64(column)*(viaDiameterMM+request.Rules.ViaClearanceMM)
		searchOffset := Point{YMM: side.sign * (halfSpanMM + escapeDistanceMM)}
		if rawAxisX {
			searchOffset = Point{XMM: side.sign * (halfSpanMM + escapeDistanceMM)}
		}
		searchX, searchY := kicadfiles.RotateBoardLocalXY(searchOffset.XMM, searchOffset.YMM, rotationDeg)
		searchPoint := Point{XMM: center.XMM + searchX, YMM: center.YMM + searchY}
		searchPoint = grid.ToPoint(grid.ToGrid(searchPoint, 0))
		probe := Segment{Start: searchPoint, End: searchPoint}
		for _, candidate := range peers {
			if strings.EqualFold(strings.TrimSpace(candidate.pad.Name), strings.TrimSpace(pad.Name)) || sameOccupancyNet(candidate.pad.Net, pad.Net) {
				continue
			}
			clearanceMM := clearances.pad(pad.Net, clearanceObjectVia, candidate.pad)
			distanceMM := segmentShapeDistance(probe, padRect(component, candidate.pad))
			viaDiameterMM = min(viaDiameterMM, 2*(distanceMM-clearanceMM-distanceEpsilon))
		}
		if viaDiameterMM <= request.Rules.TraceWidthMM+distanceEpsilon {
			continue
		}
		points = append(points, AccessPoint{
			Endpoint:    Endpoint{Ref: component.Ref, Pin: pad.Name},
			Point:       Point{XMM: center.XMM + edgeX, YMM: center.YMM + edgeY},
			SearchPoint: &searchPoint,
			Layer:       layers[0],
		})
		if minViaDiameterMM == 0 || viaDiameterMM < minViaDiameterMM {
			minViaDiameterMM = viaDiameterMM
		}
	}
	return points, minViaDiameterMM, len(points) != 0
}

func crowdedEndpointViaRules(rules Rules, diameters map[endpointID]float64) Rules {
	diameterMM := rules.ViaDiameterMM
	for _, candidate := range diameters {
		if candidate > 0 && (diameterMM <= 0 || candidate < diameterMM) {
			diameterMM = candidate
		}
	}
	if diameterMM <= 0 || diameterMM >= rules.ViaDiameterMM {
		return rules
	}
	ratio := 0.5
	if rules.ViaDiameterMM > 0 && rules.ViaDrillMM > 0 {
		ratio = rules.ViaDrillMM / rules.ViaDiameterMM
	}
	rules.ViaDiameterMM = diameterMM
	rules.ViaDrillMM = min(rules.ViaDrillMM, diameterMM*ratio)
	return rules
}

func crowdedSMDPadSideFor(pad Pad) (crowdedSMDPadSide, bool, bool) {
	if pad.Type != PadSMD || strings.TrimSpace(pad.Net) == "" || math.Abs(pad.Size.WidthMM-pad.Size.HeightMM) <= distanceEpsilon {
		return crowdedSMDPadSide{}, false, false
	}
	rawAxisX := pad.Size.WidthMM > pad.Size.HeightMM
	axisX := rawAxisX
	if int(math.Round(pad.RotationDeg/90))%2 != 0 {
		axisX = !axisX
	}
	position := pad.Position.YMM
	if axisX {
		position = pad.Position.XMM
	}
	if math.Abs(position) <= distanceEpsilon {
		return crowdedSMDPadSide{}, false, false
	}
	return crowdedSMDPadSide{axisX: axisX, sign: math.Copysign(1, position)}, rawAxisX, true
}

func filterPhysicalEndpointAccess(access PadAccess, request Request, netName string, endpoints []Endpoint) PadAccess {
	clearances := newClearancePolicy(request)
	filtered := clonePadAccessPoints(access)
	for _, endpoint := range endpoints {
		key := endpointKey(endpoint.Ref, endpoint.Pin)
		points, ok := AccessPointsForEndpoint(filtered, endpoint)
		if !ok {
			continue
		}
		safe := make([]AccessPoint, 0, len(points))
		for _, point := range points {
			probe := Segment{
				Net:     netName,
				Layer:   point.Layer,
				Start:   point.Point,
				End:     accessSearchPoint(point),
				WidthMM: request.Rules.TraceWidthMM,
			}
			clear := true
			for _, component := range request.Components {
				for _, pad := range component.Pads {
					if sameOccupancyNet(pad.Net, netName) || !padAppliesToCopperLayer(pad, point.Layer, request.Board.Layers) {
						continue
					}
					clearanceMM := clearances.pad(netName, clearanceObjectTrace, pad)
					if segmentShapeDistance(probe, padRect(component, pad))-probe.WidthMM/2 < clearanceMM-distanceEpsilon {
						clear = false
						break
					}
				}
				if !clear {
					break
				}
			}
			if clear && point.SearchPoint != nil {
				for _, obstacle := range request.Obstacles {
					if obstacle.Layer != "" && normalizeLayer(obstacle.Layer) != normalizeLayer(point.Layer) {
						continue
					}
					clearanceMM := clearances.obstacle(netName, clearanceObjectTrace, obstacle)
					if segmentShapeDistance(probe, obstacle.Geometry)-probe.WidthMM/2 < clearanceMM-distanceEpsilon {
						clear = false
						break
					}
				}
			}
			if clear && point.SearchPoint != nil {
				for _, copper := range request.Existing {
					if sameOccupancyNet(copper.Net, netName) || !existingCopperAppliesToLayer(copper, point.Layer) {
						continue
					}
					clearanceMM := clearances.pair(netName, clearanceObjectTrace, copper.Net, existingCopperKind(copper.Kind))
					if segmentShapeDistance(probe, copper.Geometry)-probe.WidthMM/2 < clearanceMM-distanceEpsilon {
						clear = false
						break
					}
				}
			}
			if clear {
				safe = append(safe, point)
			}
		}
		if len(safe) == 0 {
			delete(filtered.AccessPoints, key)
			continue
		}
		filtered.AccessPoints[key] = safe
	}
	return filtered
}

func existingCopperAppliesToLayer(copper ExistingCopper, layer string) bool {
	if copper.Kind == CopperVia || strings.TrimSpace(copper.Layer) == "" {
		return true
	}
	return normalizeLayer(copper.Layer) == normalizeLayer(layer)
}

func pinPathEndpointAccess(access *PadAccess, path GridPath, endpoint Endpoint, pointIndex int) bool {
	if access == nil || pointIndex < 0 || pointIndex >= len(path.Points) || pointIndex >= len(path.Coordinates) {
		return false
	}
	points, ok := AccessPointsForEndpoint(*access, endpoint)
	if !ok {
		return false
	}
	targetPoint := roundPoint(path.Points[pointIndex])
	targetLayerName, hasTargetLayer := path.LayerNames[path.Coordinates[pointIndex].Layer]
	targetLayer := normalizeLayer(targetLayerName)
	if !hasTargetLayer || targetLayer == "" {
		targetLayer = normalizeLayer(path.Layer)
	}
	for _, point := range points {
		if roundPoint(point.Point) == targetPoint && normalizeLayer(point.Layer) == targetLayer {
			access.AccessPoints[endpointKey(endpoint.Ref, endpoint.Pin)] = []AccessPoint{point}
			return true
		}
	}
	return false
}

func routeEndpointAccessIssue(netName string, pairIndex int, pair EndpointPair) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     fmt.Sprintf("nets[%q].pairs[%d]", netName, pairIndex),
		Message: fmt.Sprintf(
			"routed path between %s.%s and %s.%s does not terminate on known physical pad access",
			pair.From.Ref, pair.From.Pin, pair.To.Ref, pair.To.Pin,
		),
		Refs:       []string{pair.From.Ref, pair.To.Ref},
		Nets:       []string{netName},
		Suggestion: "verify pad access points and routed path endpoint alignment",
	}
}

func endpointNeckdownTrunkIssue(netName string, pairIndex int, pair EndpointPair) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     fmt.Sprintf("nets[%q].pairs[%d]", netName, pairIndex),
		Message: fmt.Sprintf(
			"endpoint neckdown path between %s.%s and %s.%s does not leave a clearance-safe full-width trunk",
			pair.From.Ref, pair.From.Pin, pair.To.Ref, pair.To.Pin,
		),
		Refs:       []string{pair.From.Ref, pair.To.Ref},
		Nets:       []string{netName},
		Suggestion: "increase endpoint access space or move the connected components farther apart",
	}
}

func routeCanceledResult(ctx context.Context) (Result, bool) {
	if err := ctx.Err(); err != nil {
		return Result{
			Status: StatusBlocked,
			Issues: []reports.Issue{routeCanceledIssue(err)},
		}, true
	}
	return Result{}, false
}

func routeCanceledIssue(err error) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeOperationCanceled,
		Severity: reports.SeverityBlocked,
		Message:  err.Error(),
	}
}

func routePairPath(ctx context.Context, request Request, access PadAccess, occupancy Occupancy, viaOccupancy Occupancy, netName string, pair EndpointPair) (GridPath, []reports.Issue) {
	if request.Strategy.Mode == ModeSingleLayer {
		return routeSingleLayerPath(ctx, request, access, occupancy, netName, pair, request.Rules.PreferLayer)
	}
	return routeTwoLayerPath(ctx, request, access, occupancy, viaOccupancy, netName, pair)
}

func routePairPathWithForcedEndpointVias(ctx context.Context, request Request, access PadAccess, occupancy Occupancy, viaOccupancy Occupancy, netName string, pair EndpointPair, forced map[endpointID]float64) (GridPath, []reports.Issue) {
	if len(forced) == 0 || request.Strategy.Mode == ModeSingleLayer {
		return routePairPath(ctx, request, access, occupancy, viaOccupancy, netName, pair)
	}
	return routeTwoLayerPathWithForcedEndpointVias(ctx, request, access, occupancy, viaOccupancy, netName, pair, forced)
}

func applyEffectiveRule(rules Rules, effective pcbrules.EffectiveRule) Rules {
	if effective.TraceWidthMM > 0 {
		rules.TraceWidthMM = effective.TraceWidthMM
	}
	if effective.ClearanceMM > 0 {
		rules.ClearanceMM = effective.ClearanceMM
	}
	if effective.ViaDiameterMM > 0 {
		rules.ViaDiameterMM = effective.ViaDiameterMM
	}
	if effective.ViaDrillMM > 0 {
		rules.ViaDrillMM = effective.ViaDrillMM
	}
	if effective.ViaClearanceMM > 0 {
		rules.ViaClearanceMM = effective.ViaClearanceMM
	}
	if effective.MaxViasPerNet > 0 {
		rules.MaxViasPerNet = effective.MaxViasPerNet
	}
	if effective.NeckdownWidthMM > 0 {
		rules.NeckdownWidthMM = effective.NeckdownWidthMM
	}
	if effective.NeckdownLengthMM > 0 {
		rules.NeckdownLengthMM = effective.NeckdownLengthMM
	}
	if effective.PreferLayer != "" {
		rules.PreferLayer = effective.PreferLayer
	}
	if len(effective.AllowedLayers) != 0 {
		rules.AllowedLayers = append([]string(nil), effective.AllowedLayers...)
	}
	return rules
}

func routingSearchRequest(request Request) Request {
	if request.Rules.NeckdownWidthMM > 0 && request.Rules.NeckdownLengthMM > 0 && request.Rules.NeckdownWidthMM < request.Rules.TraceWidthMM {
		request.Rules.TraceWidthMM = request.Rules.NeckdownWidthMM
	}
	return request
}

func applyAutomaticEndpointNeckdown(rules Rules, role NetRole, narrowEndpoint bool) Rules {
	if !narrowEndpoint || role != NetPower && role != NetGround && role != NetHighCurrent || rules.NeckdownWidthMM > 0 {
		return rules
	}
	widthMM := max(pcbrules.DefaultPowerNeckdownWidthMM, rules.MinNeckdownWidthMM)
	if widthMM >= rules.TraceWidthMM {
		return rules
	}
	rules.NeckdownWidthMM = widthMM
	rules.NeckdownLengthMM = pcbrules.DefaultPowerNeckdownLengthMM
	return rules
}

func netHasNarrowEndpoint(request Request, net Net) bool {
	for _, endpoint := range net.Endpoints {
		for _, component := range request.Components {
			if !strings.EqualFold(strings.TrimSpace(component.Ref), strings.TrimSpace(endpoint.Ref)) {
				continue
			}
			for _, pad := range component.Pads {
				if !strings.EqualFold(strings.TrimSpace(pad.Name), strings.TrimSpace(endpoint.Pin)) {
					continue
				}
				minimumPadDimensionMM := min(pad.Size.WidthMM, pad.Size.HeightMM)
				if minimumPadDimensionMM > 0 && minimumPadDimensionMM+distanceEpsilon < request.Rules.TraceWidthMM {
					return true
				}
				if len(routableLayerNames(request.Board.Layers)) >= 2 && request.Rules.MaxViasPerNet >= 2 {
					if _, _, crowded := crowdedSMDPadViaAccessPoints(component, pad, request); crowded {
						return true
					}
				}
			}
		}
	}
	return false
}

func endpointNeckdownFallbackRequest(request Request, rules Rules, role NetRole, allowSignal bool) (Request, bool) {
	if !allowSignal && role != NetPower && role != NetGround {
		return Request{}, false
	}
	widthMM := max(pcbrules.DefaultPowerNeckdownWidthMM, rules.MinNeckdownWidthMM)
	if widthMM >= rules.TraceWidthMM {
		return Request{}, false
	}
	request.Rules = rules
	request.Rules.TraceWidthMM = widthMM
	return request, true
}

func buildRouteOccupancy(request Request, netName string) (Occupancy, Occupancy, error) {
	if request.Strategy.Mode == ModeSingleLayer {
		occupancy, err := BuildOccupancy(request, netName)
		return occupancy, Occupancy{}, err
	}
	return BuildTraceAndViaOccupancy(request, netName)
}

func nominalSegmentsClearOccupancy(segments []Segment, nominalWidthMM float64, occupancy Occupancy, layers []Layer) bool {
	return segmentsClearOccupancyAtLeastWidth(segments, nominalWidthMM, occupancy, layers)
}

func segmentsClearOccupancyAtLeastWidth(segments []Segment, minimumWidthMM float64, occupancy Occupancy, layers []Layer) bool {
	gridMM := occupancy.Grid.spacingMM()
	if gridMM <= 0 {
		return false
	}
	layerIndexes, _ := LayerIndexes(layers)
	for _, segment := range segments {
		if segment.WidthMM+distanceEpsilon < minimumWidthMM {
			continue
		}
		layerIndex, ok := layerIndexes[normalizeLayer(segment.Layer)]
		if !ok {
			return false
		}
		lengthMM := pointDistance(segment.Start, segment.End)
		steps := max(1, int(math.Ceil(lengthMM/(gridMM/2))))
		for step := 0; step <= steps; step++ {
			point := interpolateSegmentPoint(segment, float64(step)/float64(steps))
			if occupancy.BlockedCell(occupancy.Grid.ToGrid(point, layerIndex)) {
				return false
			}
		}
	}
	return true
}

func extendEndpointNeckdownToClearTrunk(path GridPath, nominalWidthMM float64, neckdownWidthMM float64, initialLengthMM float64, occupancy Occupancy, layers []Layer, existing []Segment) ([]Segment, Metrics, bool) {
	base, _ := BuildSegmentsFromPath(path, nominalWidthMM)
	totalLengthMM := segmentLengthTotal(base)
	if neckdownWidthMM <= 0 || neckdownWidthMM >= nominalWidthMM || totalLengthMM <= distanceEpsilon {
		return nil, Metrics{}, false
	}
	stepMM := occupancy.Grid.spacingMM()
	if stepMM <= 0 {
		stepMM = DefaultRules().GridMM
	}
	startLengthMM := max(initialLengthMM+stepMM, stepMM)
	maximumLengthMM := totalLengthMM / 2
	tryLength := func(lengthMM float64) ([]Segment, Metrics, bool) {
		segments, metrics := BuildSegmentsFromPathWithNeckdown(path, nominalWidthMM, neckdownWidthMM, lengthMM)
		if !nominalSegmentsClearOccupancy(segments, nominalWidthMM, occupancy, layers) {
			return nil, Metrics{}, false
		}
		if segmentsContainNominalWidth(segments, nominalWidthMM) || segmentsContainNominalWidth(existing, nominalWidthMM) {
			return segments, metrics, true
		}
		return nil, Metrics{}, false
	}
	for lengthMM := startLengthMM; lengthMM < maximumLengthMM-distanceEpsilon; lengthMM += stepMM {
		if segments, metrics, ok := tryLength(lengthMM); ok {
			return segments, metrics, true
		}
	}
	if segments, metrics, ok := tryLength(maximumLengthMM); ok {
		return segments, metrics, true
	}
	return nil, Metrics{}, false
}

func endpointNeckdownAwaitingNetTrunk(path GridPath, nominalWidthMM, neckdownWidthMM float64, occupancy Occupancy, layers []Layer) ([]Segment, Metrics, bool) {
	base, _ := BuildSegmentsFromPath(path, nominalWidthMM)
	totalLengthMM := segmentLengthTotal(base)
	if neckdownWidthMM <= 0 || neckdownWidthMM >= nominalWidthMM || totalLengthMM <= distanceEpsilon {
		return nil, Metrics{}, false
	}
	segments, metrics := BuildSegmentsFromPathWithNeckdown(path, nominalWidthMM, neckdownWidthMM, totalLengthMM/2)
	if segmentsContainNominalWidth(segments, nominalWidthMM) || !nominalSegmentsClearOccupancy(segments, nominalWidthMM, occupancy, layers) {
		return nil, Metrics{}, false
	}
	return segments, metrics, true
}

func segmentsContainNominalWidth(segments []Segment, nominalWidthMM float64) bool {
	for _, segment := range segments {
		if segment.WidthMM+distanceEpsilon >= nominalWidthMM {
			return true
		}
	}
	return false
}

func segmentsUseNeckdown(segments []Segment, nominalWidthMM float64) bool {
	for _, segment := range segments {
		if segment.WidthMM+distanceEpsilon < nominalWidthMM {
			return true
		}
	}
	return false
}

func lengthPolicyIssues(netName string, effective pcbrules.EffectiveRule, route Route) []reports.Issue {
	length := routeLength(route)
	if length <= 0 {
		return nil
	}
	issues := []reports.Issue{}
	if effective.MaxLengthMM > 0 && length > effective.MaxLengthMM {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityBlocked,
			Path:       "nets." + netName + ".max_length_mm",
			Message:    "route length exceeds maximum",
			Nets:       []string{netName},
			Suggestion: "move components closer, allow a shorter layer transition, or increase max length",
		})
	}
	if effective.WarningLengthMM > 0 && length > effective.WarningLengthMM {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityWarning,
			Path:       "nets." + netName + ".warning_length_mm",
			Message:    "route length exceeds warning threshold",
			Nets:       []string{netName},
			Suggestion: "review placement or route policy for a shorter path",
		})
	}
	return issues
}

func hasBlockingIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}

func collectRouteIssues(routes []Route) []reports.Issue {
	count := 0
	for _, route := range routes {
		count += len(route.Issues)
	}
	issues := make([]reports.Issue, 0, count)
	for _, route := range routes {
		issues = append(issues, route.Issues...)
	}
	return issues
}

func existingCopperForSegments(segments []Segment) []ExistingCopper {
	existing := make([]ExistingCopper, 0, len(segments))
	for _, segment := range segments {
		radius := segment.WidthMM / 2
		existing = append(existing, ExistingCopper{
			Kind:     CopperSegment,
			Net:      segment.Net,
			Layer:    segment.Layer,
			Geometry: segmentGeometry(segment, radius),
		})
	}
	return existing
}

func segmentGeometry(segment Segment, radius float64) Shape {
	dx := segment.End.XMM - segment.Start.XMM
	dy := segment.End.YMM - segment.Start.YMM
	length := math.Sqrt(dx*dx + dy*dy)
	if length <= distanceEpsilon {
		return Shape{Rect: &Rect{
			Min: Point{XMM: segment.Start.XMM - radius, YMM: segment.Start.YMM - radius},
			Max: Point{XMM: segment.Start.XMM + radius, YMM: segment.Start.YMM + radius},
		}}
	}
	nx := -dy / length * radius
	ny := dx / length * radius
	ux := dx / length * radius
	uy := dy / length * radius
	polygon := []Point{
		{XMM: segment.Start.XMM - ux + nx, YMM: segment.Start.YMM - uy + ny},
		{XMM: segment.End.XMM + ux + nx, YMM: segment.End.YMM + uy + ny},
		{XMM: segment.End.XMM + ux - nx, YMM: segment.End.YMM + uy - ny},
		{XMM: segment.Start.XMM - ux - nx, YMM: segment.Start.YMM - uy - ny},
	}
	bounds := polygonBounds(polygon)
	return Shape{Rect: &bounds, Polygon: polygon}
}

func polygonBounds(points []Point) Rect {
	if len(points) == 0 {
		return Rect{}
	}
	bounds := Rect{Min: points[0], Max: points[0]}
	for _, point := range points[1:] {
		bounds.Min.XMM = min(bounds.Min.XMM, point.XMM)
		bounds.Min.YMM = min(bounds.Min.YMM, point.YMM)
		bounds.Max.XMM = max(bounds.Max.XMM, point.XMM)
		bounds.Max.YMM = max(bounds.Max.YMM, point.YMM)
	}
	return bounds
}

func existingCopperForVias(vias []Via, boardLayers []Layer) []ExistingCopper {
	physicalLayers := make([]string, 0, len(boardLayers))
	seenLayers := map[string]struct{}{}
	for _, layer := range boardLayers {
		if layer.Kind != LayerCopper {
			continue
		}
		key := normalizeLayer(layer.Name)
		if key == "" {
			continue
		}
		if _, ok := seenLayers[key]; ok {
			continue
		}
		seenLayers[key] = struct{}{}
		physicalLayers = append(physicalLayers, layer.Name)
	}
	existing := make([]ExistingCopper, 0, len(vias)*max(1, len(physicalLayers)))
	for _, via := range vias {
		radius := via.DiameterMM / 2
		layers := physicalLayers
		if len(layers) == 0 {
			layers = via.Layers
		}
		for _, layer := range layers {
			existing = append(existing, ExistingCopper{
				Kind:  CopperVia,
				Net:   via.Net,
				Layer: layer,
				Geometry: Shape{Rect: &Rect{
					Min: Point{XMM: via.At.XMM - radius, YMM: via.At.YMM - radius},
					Max: Point{XMM: via.At.XMM + radius, YMM: via.At.YMM + radius},
				}},
			})
		}
	}
	return existing
}
