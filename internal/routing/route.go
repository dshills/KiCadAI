package routing

import (
	"context"
	"fmt"
	"math"

	"kicadai/internal/pcbrules"
	"kicadai/internal/reports"
)

const routePairContextCheckInterval = 16

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
		var fallbackRequest Request
		var fallbackOccupancy Occupancy
		var fallbackViaOccupancy Occupancy
		fallbackReady := false
		for pairIndex, pair := range plan.Pairs {
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
			path, routeIssues := routePairPath(ctx, searchRequest, access, occupancy, viaOccupancy, plan.Net.Name, pair)
			route.SearchNodes += path.SearchNodes
			result.Metrics.SearchNodes += path.SearchNodes
			if path.SearchLimitHit {
				route.SearchLimitHit = true
				result.Metrics.MaxSearchNodesHit = true
			}
			neckdownWidthMM := netRequest.Rules.NeckdownWidthMM
			neckdownLengthMM := netRequest.Rules.NeckdownLengthMM
			if len(routeIssues) != 0 && neckdownWidthMM == 0 {
				candidate, ok := endpointNeckdownFallbackRequest(netRequest, netRequest.Rules, plan.Net.Role)
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
						fallbackPath, fallbackIssues := routePairPath(ctx, fallbackRequest, access, fallbackOccupancy, fallbackViaOccupancy, plan.Net.Name, pair)
						route.SearchNodes += fallbackPath.SearchNodes
						result.Metrics.SearchNodes += fallbackPath.SearchNodes
						if len(fallbackIssues) == 0 {
							path = fallbackPath
							routeIssues = nil
							neckdownWidthMM = fallbackRequest.Rules.TraceWidthMM
							neckdownLengthMM = pcbrules.DefaultPowerNeckdownLengthMM
						}
					}
				}
			}
			var segments []Segment
			var metrics Metrics
			if len(routeIssues) == 0 {
				segments, metrics = BuildSegmentsFromPathWithNeckdown(path, netRequest.Rules.TraceWidthMM, neckdownWidthMM, neckdownLengthMM)
				if segmentsUseNeckdown(segments, netRequest.Rules.TraceWidthMM) && !nominalSegmentsClearOccupancy(segments, netRequest.Rules.TraceWidthMM, nominalOccupancy, netRequest.Board.Layers) {
					routeIssues = []reports.Issue{endpointNeckdownTrunkIssue(plan.Net.Name, pairIndex, pair)}
				}
			}
			if len(routeIssues) != 0 {
				route.Issues = append(route.Issues, routeIssues...)
				netFailed = true
				failed = true
				break
			}
			vias := BuildViasFromPath(path, netRequest.Rules)
			route.Segments = append(route.Segments, segments...)
			route.Vias = append(route.Vias, vias...)
			netSegmentCount += len(segments)
			netViaCount += len(vias)
			netLengthMM += metrics.TotalLengthMM
			request.Existing = append(request.Existing, existingCopperForSegments(segments)...)
			request.Existing = append(request.Existing, existingCopperForVias(vias)...)
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

func endpointNeckdownFallbackRequest(request Request, rules Rules, role NetRole) (Request, bool) {
	if role != NetPower && role != NetGround {
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
	gridMM := occupancy.Grid.spacingMM()
	if gridMM <= 0 {
		return false
	}
	layerIndexes, _ := LayerIndexes(layers)
	for _, segment := range segments {
		if segment.WidthMM+distanceEpsilon < nominalWidthMM {
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

func existingCopperForVias(vias []Via) []ExistingCopper {
	existing := make([]ExistingCopper, 0, len(vias)*2)
	for _, via := range vias {
		radius := via.DiameterMM / 2
		for _, layer := range via.Layers {
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
