package routing

import (
	"context"
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
		if plan.Net.Class == "" && (plan.Net.Role == NetPower || plan.Net.Role == NetGround || plan.Net.Role == NetHighCurrent) {
			route.Issues = append(route.Issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityWarning,
				Path:       "nets." + plan.Net.Name + ".class",
				Message:    "power or high-current net has no explicit net class",
				Nets:       []string{plan.Net.Name},
				Suggestion: "assign a net class with explicit trace, via, and clearance rules",
			})
		}
		var occupancy Occupancy
		if !netFailed {
			var err error
			occupancy, err = BuildOccupancy(netRequest, plan.Net.Name)
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
		} else {
			failed = true
		}
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
			path, routeIssues := routePairPath(ctx, netRequest, access, occupancy, plan.Net.Name, pair)
			route.SearchNodes += path.SearchNodes
			result.Metrics.SearchNodes += path.SearchNodes
			if path.SearchLimitHit {
				route.SearchLimitHit = true
				result.Metrics.MaxSearchNodesHit = true
			}
			if len(routeIssues) != 0 {
				route.Issues = append(route.Issues, routeIssues...)
				netFailed = true
				failed = true
				break
			}
			segments, metrics := BuildSegmentsFromPath(path, netRequest.Rules.TraceWidthMM)
			vias := BuildViasFromPath(path, netRequest.Rules)
			route.Segments = append(route.Segments, segments...)
			route.Vias = append(route.Vias, vias...)
			result.Metrics.SegmentCount += len(segments)
			result.Metrics.ViaCount += len(vias)
			result.Metrics.TotalLengthMM = roundMM(result.Metrics.TotalLengthMM + metrics.TotalLengthMM)
			request.Existing = append(request.Existing, existingCopperForSegments(segments)...)
			request.Existing = append(request.Existing, existingCopperForVias(vias)...)
		}
		if netFailed || len(route.Issues) != 0 {
			route.Status = RouteStatusFailed
			result.Metrics.FailedNetCount++
			result.Routes = append(result.Routes, route)
			if !request.Strategy.AllowPartial {
				result.Status = StatusBlocked
				result.Issues = append(issues, collectRouteIssues(result.Routes)...)
				quality := BuildQualityReport(request, result)
				result.Quality = &quality
				return result
			}
			continue
		}
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
		result.Issues = append(result.Issues, operationIssues...)
		if result.Status == StatusRouted {
			result.Status = StatusBlocked
		}
	}
	quality := BuildQualityReport(request, result)
	result.Quality = &quality
	return result
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

func routePairPath(ctx context.Context, request Request, access PadAccess, occupancy Occupancy, netName string, pair EndpointPair) (GridPath, []reports.Issue) {
	if request.Strategy.Mode == ModeSingleLayer {
		return routeSingleLayerPath(ctx, request, access, occupancy, netName, pair, request.Rules.PreferLayer)
	}
	return routeTwoLayerPath(ctx, request, access, occupancy, netName, pair)
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
	if effective.PreferLayer != "" {
		rules.PreferLayer = effective.PreferLayer
	}
	if len(effective.AllowedLayers) != 0 {
		rules.AllowedLayers = append([]string(nil), effective.AllowedLayers...)
	}
	return rules
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
